package dogstatsd

// A word about the dogstatsd plugin implementation
// ------------------------------------------------
//
// Prometheus is used to collect metrics across coredns. In order to publish
// those metrics to a dogstatsd agent, the plugin acts as an internal collector
// that scraps the metrics at regular interval (just like prometheus would do),
// and publish them to the datadog agent.
//
// The complex parts about bridging between prometheus and dogstatsd are the
// subtle variations in how they implement similar concepts. For example, in
// prometheus counters are always incrementing values, but in dogstatsd only
// the increments are published. Same goes with the summaries and historigrams
// of prometheus, which are merged into a single histogram concept in dogstatsd.
//
// In order to provide meaningful insights, the translation layer has to
// remember the state of the previous iteration in order to compute values to
// push to the dogstatsd agent.

import (
	"context"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Dogstatsd is the implementation of the dogstasd coredns plugin.
type Dogstatsd struct {
	Next plugin.Handler

	// Address of the dogstatsd agent to push metrics to.
	Addr string

	// Size of the socket buffer used to push metrics to the dogstatsd agent.
	BufferSize int

	// Time interval between flushes of metrics to the dogstasd agent.
	FlushInterval time.Duration

	// Reg is the prometheus registry used by the metrics plugin where all
	// metrics are registered.
	Reg *prometheus.Registry

	// Those flags control whether this plugin instance is allowed to report
	// go and process metrics. This is used because those metrics are global
	// so it is OK if only a single plugin pushes them to the dogstatsd agent.
	EnableGoMetrics      bool
	EnableProcessMetrics bool

	// ZoneNames is the list of zones that this plugin reports metrics for.
	ZoneNames []string

	once   sync.Once
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	zones  map[string]struct{}

	dockerClient dockerClient
	dockerCache  atomic.Value

	clients   counterStore
	names     counterStore
	exchanges counterStore

	// Generates a random float64 value between min and max. It's made
	// configurable so it can be mocked during tests.
	randFloat64 func(min, max float64) float64
}

const (
	defaultAddr          = "udp://localhost:8125"
	defaultBufferSize    = 1024
	defaultFlushInterval = 1 * time.Minute
)

// New returns a new instance of a dogstatsd plugin.
func New() *Dogstatsd {
	return &Dogstatsd{
		Addr:          defaultAddr,
		BufferSize:    defaultBufferSize,
		FlushInterval: defaultFlushInterval,

		dockerClient: dockerClient{
			host: os.Getenv("DOCKER_HOST"),
		},

		clients:   makeCounterStore(),
		names:     makeCounterStore(),
		exchanges: makeCounterStore(),
	}
}

// Name returns the name of the plugin.
func (d *Dogstatsd) Name() string { return "dogstatsd" }

// ServeDNS satisfies the plugin.Handler interface.
func (d *Dogstatsd) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if cache, ok := d.dockerCache.Load().(map[string][]string); ok {
		addr := w.RemoteAddr().String()
		addr, _, _ = net.SplitHostPort(addr)
		// If we have one or more client registered for the address we increment
		// the corresponding counters.
		for _, a := range cache[addr] {
			d.clients.incr(a)
			d.exchanges.incr(a + "/" + r.Question[0].Name)
		}
	}

	d.names.incr(r.Question[0].Name)
	return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
}

// Start the dogstatsd plugin. The method returns immediatly after starting the
// plugin's internal goroutine.
func (d *Dogstatsd) Start() {
	d.once.Do(d.init)
	d.wg.Add(1)
	go d.run(d.ctx)
}

// Stop interrupts the runing plugin.
func (d *Dogstatsd) Stop() {
	d.once.Do(d.init)
	d.cancel()
	d.wg.Wait()
}

func (d *Dogstatsd) init() {
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.zones = make(map[string]struct{}, len(d.ZoneNames))

	for _, zone := range d.ZoneNames {
		d.zones[zone] = struct{}{}
	}
}

func (d *Dogstatsd) run(ctx context.Context) {
	defer d.wg.Done()
	log.Printf("[INFO] dogstatsd %s { buffer %d; flush %s; go %t; process %t; zones %s }", d.Addr, d.BufferSize, d.FlushInterval, d.EnableGoMetrics, d.EnableProcessMetrics, d.ZoneNames)

	ticker := time.NewTicker(d.FlushInterval)
	defer ticker.Stop()

	state := make(state)
	for {
		d.refreshDockerCache()
		d.reportMetrics(state)
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dogstatsd) refreshDockerCache() {
	containers, err := d.dockerClient.listContainers()

	if err != nil {
		log.Printf("[ERROR] failed to list containers from docker at %s: %s", d.dockerClient.host, err)
		return
	}

	cache := map[string][]string{}

	for _, container := range containers {
		for _, network := range container.NetworkSettings.Networks {
			imageName := container.Image.name()
			ipAddress := network.IPAddress
			if len(ipAddress) == 0 {
				ipAddress = network.IPAMConfig.IPv4Address
			}
			if len(ipAddress) == 0 {
				ipAddress = network.IPAMConfig.IPv6Address
			}
			if len(ipAddress) != 0 {
				cache[ipAddress] = append(cache[ipAddress], imageName)
				log.Printf("[INFO] update docker cache: %s->%s", imageName, ipAddress)
			}
		}
	}

	d.dockerCache.Store(cache)
}

func (d *Dogstatsd) reportMetrics(state state) {
	metrics, err := d.collectMetrics(state)

	if err != nil {
		log.Printf("[ERROR] collecting metrics: %s", err)
		return
	}

	log.Printf("[INFO] flushing %d metrics to %s", len(metrics), d.Addr)

	if err := d.flushMetrics(metrics); err != nil {
		log.Printf("[ERROR] flushing metrics to the dogstatsd agent at %s: %s", d.Addr, err)
	}
}

func (d *Dogstatsd) collectMetrics(state state) ([]metric, error) {
	metricFamilies, err := d.Reg.Gather()
	if err != nil {
		return nil, err
	}

	metrics := make([]metric, 0, 2*len(metricFamilies))
	rand := d.randFloat64
	if rand == nil {
		rand = randFloat64
	}

	for _, f := range metricFamilies {
		if !d.EnableGoMetrics && isGoMetric(*f.Name) {
			continue
		}

		if !d.EnableProcessMetrics && isProcessMetric(*f.Name) {
			continue
		}

		for _, m := range f.Metric {
			if !d.matchZones(m) {
				continue
			}

			for _, v := range makeMetrics(f, m, rand) {
				if v, ok := state.observe(v); ok {
					metrics = append(metrics, v)
				}
			}
		}
	}

	for _, c := range d.clients.top(10) {
		metrics = append(metrics, c.metric("coredns.dns.clients.top10", "client"))
	}

	for _, c := range d.names.top(10) {
		metrics = append(metrics, c.metric("coredns.dns.names.top10", "name"))
	}

	for _, c := range d.exchanges.top(10) {
		metrics = append(metrics, c.metric("coredns.dns.exchanges.top10", "exchange"))
	}

	return metrics, nil
}

func (d *Dogstatsd) matchZones(m *dto.Metric) bool {
	hasZone := false

	if len(d.ZoneNames) == 0 {
		return true // no zones configured, match all
	}

	for _, label := range m.Label {
		if *label.Name == "zone" {
			hasZone = true
			if _, match := d.zones[*label.Value]; match {
				return true
			}
		}
	}

	return !hasZone // no zones on the metric? OK
}

func (d *Dogstatsd) flushMetrics(metrics []metric) error {
	conn, bufferSize, err := dial(d.Addr, d.BufferSize)
	if err != nil {
		return err
	}
	defer conn.Close()

	out := make([]byte, 0, bufferSize)
	buf := make([]byte, 0, bufferSize)

	for _, m := range metrics {
		buf = appendMetric(buf[:0], m)

		if len(buf) > bufferSize {
			log.Printf("[WARN] dogstatsd metric of size %d B exceeds the configured buffer size of %d B", len(buf), bufferSize)
			continue
		}

		if (len(out) + len(buf)) > bufferSize {
			if _, err := conn.Write(out); err != nil {
				return err
			}
			out = out[:0]
		}

		out = append(out, buf...)
	}

	if len(out) != 0 {
		_, err = conn.Write(out)
	}
	return err
}

// taken from https://github.com/segmentio/stats/datadog
func dial(address string, bufferSizeHint int) (conn net.Conn, bufferSize int, err error) {
	var network = "udp"
	var f *os.File

	if i := strings.Index(address, "://"); i >= 0 {
		network, address = address[:i], address[i+3:]
	}

	if conn, err = net.Dial(network, address); err != nil {
		return
	}

	uc, ok := conn.(*net.UDPConn)
	if !ok {
		bufferSize = bufferSizeHint
		return
	}

	if f, err = uc.File(); err != nil {
		conn.Close()
		return
	}
	defer f.Close()
	fd := int(f.Fd())

	// The kernel refuses to send UDP datagrams that are larger than the size of
	// the size of the socket send buffer. To maximize the number of metrics
	// sent in one batch we attempt to adjust the kernel buffer size to accept
	// larger datagrams, or fallback to the default socket buffer size if it
	// failed.
	if bufferSize, err = syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF); err != nil {
		conn.Close()
		return
	}

	// The kernel applies a 2x factor on the socket buffer size, only half of it
	// is available to write datagrams from user-space, the other half is used
	// by the kernel directly.
	bufferSize /= 2

	for bufferSizeHint > bufferSize && bufferSizeHint > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, bufferSizeHint); err == nil {
			bufferSize = bufferSizeHint
			break
		}
		bufferSizeHint /= 2
	}

	// Even tho the buffer agrees to support a bigger size it shouldn't be
	// possible to send datagrams larger than 65 KB on an IPv4 socket, so let's
	// enforce the max size.
	const maxBufferSize = 65507
	if bufferSize > maxBufferSize {
		bufferSize = maxBufferSize
	}

	// Use the size hint as an upper bound, event if the socket buffer is
	// larger, this gives control in situations where the receive buffer size
	// on the other side is known but cannot be controlled so the client does
	// not produce datagrams that are too large for the receiver.
	//
	// Related issue: https://github.com/DataDog/dd-agent/issues/2638
	if bufferSize > bufferSizeHint {
		bufferSize = bufferSizeHint
	}

	// Creating the file put the socket in blocking mode, reverting.
	syscall.SetNonblock(fd, true)
	return
}

func randFloat64(min, max float64) float64 {
	return min + (rand.Float64() * (max - min))
}
