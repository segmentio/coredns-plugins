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
			"A": "hello-1",
			"B": "hello-2",
			"C": "hello-3",
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
	server, plugin, state := setupTest()
	defer server.Close()

	plugin.Reg.MustRegister(
		counter1,
		counter2,
		gauge1,
		histogram1,
	)

	for i := 0; i != 10; i++ {
		t.Logf("#%d: simple/repeat/merge", i)
		testDogstatsdSimple(t, plugin, server, state)
		testDogstatsdRepeat(t, plugin, server, state)
		testDogstatsdMerge(t, plugin, server, state)

		t.Logf("#%d: simple 2x", i)
		testDogstatsdSimple(t, plugin, server, state)
		testDogstatsdSimple(t, plugin, server, state)

		t.Logf("#%d: repeat 2x", i)
		testDogstatsdRepeat(t, plugin, server, state)
		testDogstatsdRepeat(t, plugin, server, state)

		t.Logf("#%d: merge 2x", i)
		testDogstatsdMerge(t, plugin, server, state)
		testDogstatsdMerge(t, plugin, server, state)
	}
}

func testDogstatsdSimple(t *testing.T, plugin *Dogstatsd, server server, state state) {
	t.Helper()

	counter1.Add(42)
	counter2.Add(1)
	gauge1.Set(10)
	histogram1.Observe(0)
	histogram1.Observe(42)
	histogram1.Observe(42)

	plugin.pulse(state)
	assertRead(t, server,
		"coredns.segment.counter1:42|c",
		"coredns.segment.counter2:1|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",
		"coredns.segment.histogram1:0|h",
		"coredns.segment.histogram1:40|h|@0.5",
	)
}

func testDogstatsdRepeat(t *testing.T, plugin *Dogstatsd, server server, state state) {
	t.Helper()

	for i := 0; i != 20; i++ {
		counter2.Add(float64(i))
		plugin.pulse(state)
	}

	assertRead(t, server,
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:1|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:2|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:3|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:4|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:5|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:6|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:7|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:8|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:9|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:10|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:11|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:12|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:13|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:14|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:15|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:16|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:17|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:18|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",

		"coredns.segment.counter2:19|c",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",
	)
}

func testDogstatsdMerge(t *testing.T, plugin *Dogstatsd, server server, state state) {
	t.Helper()

	for i := 0; i != 100; i++ {
		histogram1.Observe(float64(i + 1))
	}

	plugin.pulse(state)
	assertRead(t, server,
		"coredns.segment.histogram1:0|h|@0.1",
		"coredns.segment.histogram1:10|h|@0.1",
		"coredns.segment.histogram1:20|h|@0.1",
		"coredns.segment.histogram1:30|h|@0.1",
		"coredns.segment.histogram1:40|h|@0.1",
		"coredns.segment.histogram1:50|h|@0.1",
		"coredns.segment.histogram1:60|h|@0.1",
		"coredns.segment.histogram1:70|h|@0.1",
		"coredns.segment.histogram1:80|h|@0.1",
		"coredns.segment.histogram1:90|h|@0.1",
		"coredns.segment.gauge1:10|g|#a:hello-1,b:hello-2,c:hello-3",
	)
}

func TestDogstatsdGoMetrics(t *testing.T) {
	t.Run("enabled", func(t *testing.T) { testDogstatsdGoMetrics(t, true) })
	t.Run("disabled", func(t *testing.T) { testDogstatsdGoMetrics(t, false) })
}

func testDogstatsdGoMetrics(t *testing.T, enable bool) {
	server, plugin, state := setupTest()

	plugin.Reg.MustRegister(prometheus.NewGoCollector())
	plugin.EnableGoMetrics = enable

	plugin.pulse(state)
	time.Sleep(100 * time.Millisecond)
	server.Close()

	count := 0

	for packet := range server.packets {
		if !enable {
			t.Error("no go metrics must be produced when they are disabled, got", string(packet))
		} else if !strings.HasPrefix(string(packet), "coredns.go.") {
			t.Error("go metrics were enabled and an unexpected metric was received:", string(packet))
		}
		count++
	}

	if enable && count == 0 {
		t.Error("go metrics were enabled but none have been produced")
	}
}

func setupTest() (server, *Dogstatsd, state) {
	server := dogstatsdServer()
	plugin := dogstastdPlugin(server.addr())
	state := make(state)
	return server, plugin, state
}

func dogstastdPlugin(addr string) *Dogstatsd {
	plugin := New()
	plugin.Addr = addr
	plugin.BufferSize = 100 // for the purpose of the test, forbidden otherwise
	plugin.Reg = prometheus.NewRegistry()
	plugin.randFloat64 = func(min, max float64) float64 { return min }
	return plugin
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
	t.Helper()

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
