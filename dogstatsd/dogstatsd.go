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
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
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

	once   sync.Once
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
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
	}
}

// Name returns the name of the plugin.
func (d *Dogstatsd) Name() string { return "dogstatsd" }

// ServeDNS satisfies the plugin.Handler interface.
func (d *Dogstatsd) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
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
	d.cancel()
	d.wg.Wait()
}

func (d *Dogstatsd) init() {
	d.ctx, d.cancel = context.WithCancel(context.Background())
}

func (d *Dogstatsd) run(ctx context.Context) {
	defer d.wg.Done()
	log.Printf("[INFO] dogstatsd %s { buffer %d; flush %s }", d.Addr, d.BufferSize, d.FlushInterval)

	ticker := time.NewTicker(d.FlushInterval)
	defer ticker.Stop()

	state := make(state)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.pulse(state)
		}
	}
}

func (d *Dogstatsd) pulse(state state) {
	metrics, err := d.collect(state)

	if err != nil {
		log.Printf("[ERROR] collecting metrics: %s", err)
		return
	}

	log.Printf("[INFO] flushing %d metrics to %s", len(metrics), d.Addr)

	if err := d.flush(metrics); err != nil {
		log.Printf("[ERROR] flushing metrics to the dogstatsd agent at %s: %s", d.Addr, err)
	}
}

func (d *Dogstatsd) collect(state state) ([]metric, error) {
	metricFamilies, err := d.Reg.Gather()
	if err != nil {
		return nil, err
	}

	metrics := make([]metric, 0, 2*len(metricFamilies))

	for _, f := range metricFamilies {
		for _, m := range f.Metric {
			for _, v := range makeMetrics(f, m) {
				metrics = append(metrics, state.observe(v)...)
			}
		}
	}

	return metrics, nil
}

func (d *Dogstatsd) flush(metrics []metric) error {
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
