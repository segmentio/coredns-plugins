package consul

import (
	"log"
	"sync"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
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
	}, []string{"type"})

	cacheServices = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "services_total",
		Help:      "The number of elements in the cache.",
	})

	cacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "hits_total",
		Help:      "The count of cache hits.",
	}, []string{"type"})

	cacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "misses_total",
		Help:      "The count of cache misses.",
	})

	cacheEvictions = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "evictions_total",
		Help:      "The count of cache evictions.",
	})

	cachePrefetches = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "prefetch_total",
		Help:      "The number of time the cache has prefetched a cached item.",
	})

	cacheFetchSizes = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "fetch_size",
		Help:      "The distribution of response sizes to Consul requests.",
		Buckets:   []float64{1, 5, 10, 20, 50, 100, 500, 1000, 2000, 5000, 10000},
	})

	cacheFetchDurations = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "fetch_duration_seconds",
		Help:      "The distribution of response time to Consul requests.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	})
)

func cacheSizeAddSuccess(n int)    { cacheSize.WithLabelValues(success).Add(float64(n)) }
func cacheSizeAddDenial(n int)     { cacheSize.WithLabelValues(denial).Add(float64(n)) }
func cacheServicesAdd(n int)       { cacheServices.Add(float64(n)) }
func cacheHitsIncSuccess()         { cacheHits.WithLabelValues(success).Inc() }
func cacheHitsIncDenial()          { cacheHits.WithLabelValues(denial).Inc() }
func cacheMissesInc()              { cacheMisses.Inc() }
func cacheEvictionsInc()           { cacheEvictions.Inc() }
func cachePrefetchesInc()          { cachePrefetches.Inc() }
func cacheFetchSizesObserve(n int) { cacheFetchSizes.Observe(float64(n)) }
func cacheFetchDurationsObserve(d time.Duration) {
	cacheFetchDurations.Observe(float64(d) / float64(time.Second))
}

func initializeMetrics() {
	cacheSize.WithLabelValues(success)
	cacheSize.WithLabelValues(denial)
	cacheHits.WithLabelValues(success)
	cacheHits.WithLabelValues(denial)
}

func registerMetrics(c *caddy.Controller) error {
	once.Do(func() {
		if m := dnsserver.GetConfig(c).Handler("prometheus"); m == nil {
			log.Print("[WARN] metrics are disabled, do not use this configuration in production!")
		} else if r, ok := m.(*metrics.Metrics); !ok {
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
