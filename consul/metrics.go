package consul

import (
	"log"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	metricsPlugin "github.com/coredns/coredns/plugin/metrics"
	"github.com/mholt/caddy"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	consulSubsystem = "consul_cache"
	success         = "success"
	denial          = "denial"
)

var (
	once sync.Once

	cacheSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "size",
		Help:      "The number of elements in the cache.",
	}, []string{"dc", "tag", "name", "type"})

	cacheServices = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "services_total",
		Help:      "The number of elements in the cache.",
	}, []string{"dc", "tag", "name"})

	cacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "hits_total",
		Help:      "The count of cache hits.",
	}, []string{"dc", "tag", "name", "type"})

	cacheMisses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "misses_total",
		Help:      "The count of cache misses.",
	}, []string{"dc", "tag", "name"})

	cacheEvictions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "evictions_total",
		Help:      "The count of cache evictions.",
	}, []string{"dc", "tag", "name"})

	cachePrefetches = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "prefetch_total",
		Help:      "The number of time the cache has prefetched a cached item.",
	}, []string{"dc", "tag", "name"})

	cacheFetchSizes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "fetch_size",
		Help:      "The distribution of response sizes to Consul requests.",
		Buckets:   []float64{1, 5, 10, 20, 50, 100, 500, 1000, 2000, 5000, 10000},
	}, []string{"dc", "tag", "name"})

	cacheFetchDurations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "fetch_duration_seconds",
		Help:      "The distribution of response time to Consul requests.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"dc", "tag", "name"})
)

type metrics struct {
	name string
	tag  string
	dc   string
}

func (m metrics) cacheSizeAddSuccess(n int) {
	cacheSize.WithLabelValues(m.dc, m.tag, m.name, success).Add(float64(n))
}

func (m metrics) cacheSizeAddDenial(n int) {
	cacheSize.WithLabelValues(m.dc, m.tag, m.name, denial).Add(float64(n))
}

func (m metrics) cacheServicesAdd(n int) {
	cacheServices.WithLabelValues(m.dc, m.tag, m.name).Add(float64(n))
}

func (m metrics) cacheHitsIncSuccess() {
	cacheHits.WithLabelValues(m.dc, m.tag, m.name, success).Inc()
}

func (m metrics) cacheHitsIncDenial() {
	cacheHits.WithLabelValues(m.dc, m.tag, m.name, denial).Inc()
}

func (m metrics) cacheMissesInc() {
	cacheMisses.WithLabelValues(m.dc, m.tag, m.name).Inc()
}

func (m metrics) cacheEvictionsInc() {
	cacheEvictions.WithLabelValues(m.dc, m.tag, m.name).Inc()
}

func (m metrics) cachePrefetchesInc() {
	cachePrefetches.WithLabelValues(m.dc, m.tag, m.name).Inc()
}

func (m metrics) cacheFetchSizesObserve(n int) {
	cacheFetchSizes.WithLabelValues(m.dc, m.tag, m.name).Observe(float64(n))
}

func (m metrics) cacheFetchDurationsObserve(d time.Duration) {
	cacheFetchDurations.WithLabelValues(m.dc, m.tag, m.name).Observe(float64(d) / float64(time.Second))
}

func registerMetrics(c *caddy.Controller) error {
	once.Do(func() {
		if m := dnsserver.GetConfig(c).Handler("prometheus"); m == nil {
			log.Print("[WARN] metrics are disabled, do not use this configuration in production!")
		} else if r, ok := m.(*metricsPlugin.Metrics); !ok {
			log.Printf("[WARN] the registered metrics plugin is of an unexpected %T type", m)
		} else {
			r.MustRegister(cacheSize)
			r.MustRegister(cacheServices)
			r.MustRegister(cacheHits)
			r.MustRegister(cacheMisses)
			r.MustRegister(cacheEvictions)
			r.MustRegister(cachePrefetches)
			r.MustRegister(cacheFetchSizes)
			r.MustRegister(cacheFetchDurations)
		}
	})
	return nil
}
