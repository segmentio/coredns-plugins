package dogstatsd

import (
	"io/ioutil"
	"log"
	"net"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	counter1 = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coredns",
		Subsystem: "segment",
		Name:      "counter1",
		Help:      "Test counter 1.",
	})

	counter2 = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "coredns",
		Subsystem: "segment",
		Name:      "counter2",
		Help:      "Test counter 2.",
	})

	gauge1 = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coredns",
		Subsystem: "segment",
		Name:      "gauge1",
		Help:      "Test gauge 1.",
		ConstLabels: prometheus.Labels{
			"A": "1",
			"B": "2",
			"C": "3",
		},
	})

	histogram1 = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coredns",
		Subsystem: "segment",
		Name:      "histogram1",
		Help:      "Test histogram 1.",
		Buckets:   []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	})
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestDogstatsd(t *testing.T) {
	state := make(state)

	server := dogstatsdServer()
	defer server.Close()

	d := New()
	d.Addr = server.addr()
	d.BufferSize = 100 // for the purpose of the test, forbidden otherwise
	d.Reg = prometheus.NewRegistry()
	d.Reg.MustRegister(
		counter1,
		counter2,
		gauge1,
		histogram1,
	)

	// simple case
	counter1.Add(42)
	counter2.Add(1)
	gauge1.Set(10)
	histogram1.Observe(0)
	histogram1.Observe(12)
	histogram1.Observe(12)
	d.pulse(state)
	assertRead(t, server,
		"coredns.segment.counter1:42|c",
		"coredns.segment.counter2:1|c",
		"coredns.segment.gauge1:10|g|#a:1,b:2,c:3",
		"coredns.segment.histogram1:5|h",
		"coredns.segment.histogram1:12.5|h",
		"coredns.segment.histogram1:17.5|h",
	)

	// repeated changes between each pulse, first one is a no-op (so only 19
	// changes are pushed).
	for i := 0; i != 20; i++ {
		counter2.Add(float64(i))
		d.pulse(state)
	}
	assertRead(t, server,
		"coredns.segment.counter2:1|c",
		"coredns.segment.counter2:2|c",
		"coredns.segment.counter2:3|c",
		"coredns.segment.counter2:4|c",
		"coredns.segment.counter2:5|c",
		"coredns.segment.counter2:6|c",
		"coredns.segment.counter2:7|c",
		"coredns.segment.counter2:8|c",
		"coredns.segment.counter2:9|c",
		"coredns.segment.counter2:10|c",
		"coredns.segment.counter2:11|c",
		"coredns.segment.counter2:12|c",
		"coredns.segment.counter2:13|c",
		"coredns.segment.counter2:14|c",
		"coredns.segment.counter2:15|c",
		"coredns.segment.counter2:16|c",
		"coredns.segment.counter2:17|c",
		"coredns.segment.counter2:18|c",
		"coredns.segment.counter2:19|c",
	)

	for i := 0; i != 10; i++ {
		histogram1.Observe(float64(i))
	}
	d.pulse(state)
	assertRead(t, server,
		"coredns.segment.histogram1:0.5|h",
		"coredns.segment.histogram1:1.5|h",
		"coredns.segment.histogram1:2.5|h",
		"coredns.segment.histogram1:3.5|h",
		"coredns.segment.histogram1:4.5|h",
		"coredns.segment.histogram1:5.5|h",
		"coredns.segment.histogram1:6.5|h",
		"coredns.segment.histogram1:7.5|h",
		"coredns.segment.histogram1:8.5|h",
		"coredns.segment.histogram1:9.5|h",
	)
}

func dogstatsdServer() server {
	c, err := net.ListenPacket("udp", "127.0.0.1:")
	if err != nil {
		panic(err)
	}

	p := make(chan string, 1000)

	go func(p chan<- string) {
		defer close(p)
		b := make([]byte, 65536)
		c.SetReadDeadline(time.Now().Add(5 * time.Second)) // test lasts at most 5s
		for {
			n, _, err := c.ReadFrom(b)
			if err != nil {
				return
			}
			for _, line := range strings.Split(string(b[:n]), "\n") {
				if line != "" {
					p <- line
				}
			}
		}
	}(p)

	return server{PacketConn: c, packets: p}
}

type server struct {
	net.PacketConn
	packets <-chan string
}

func (s server) addr() string {
	a := s.LocalAddr()
	return a.Network() + "://" + a.String()
}

func assertRead(t *testing.T, s server, packets ...string) {
	found := make([]string, 0, len(packets))
	expected := make([]string, len(packets))
	copy(expected, packets)

	for range expected {
		p, ok := <-s.packets
		if ok {
			found = append(found, p)
		} else {
			t.Error("unexpected EOF")
			t.Logf("expected:\n%q", expected)
			t.Logf("found:\n%q", found)
			return
		}
	}

	// UDP doesn't garantee order, all we care about is getting all the packets.
	sort.Strings(found)
	sort.Strings(expected)

	if !reflect.DeepEqual(found, expected) {
		t.Errorf("packets mismatch")
		t.Logf("expected:\n%q", expected)
		t.Logf("found:\n%q", found)
	}
}
