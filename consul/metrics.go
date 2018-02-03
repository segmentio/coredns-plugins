package consul

import (
	"sync"

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

	cachePrefetches = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: consulSubsystem,
		Name:      "prefetch_total",
		Help:      "The number of time the cache has prefetched a cached item.",
	})
)

func cacheSizeAddSuccess(n int) { cacheSize.WithLabelValues(success).Set(float64(n)) }
func cacheSizeAddDenial(n int)  { cacheSize.WithLabelValues(denial).Set(float64(n)) }
func cacheServicesAdd(n int)    { cacheServices.Add(float64(n)) }
func cacheHitsIncSuccess()      { cacheHits.WithLabelValues(success).Inc() }
func cacheHitsIncDenial()       { cacheHits.WithLabelValues(denial).Inc() }
func cacheMissesInc()           { cacheMisses.Inc() }
func cachePrefetchesInc()       { cachePrefetches.Inc() }

func initializeMetrics() {
	cacheSize.WithLabelValues(success)
	cacheSize.WithLabelValues(denial)
	cacheHits.WithLabelValues(success)
	cacheHits.WithLabelValues(denial)
}

func registerMetrics(c *caddy.Controller) error {
	once.Do(func() {
		m := dnsserver.GetConfig(c).Handler("prometheus")
		if m == nil {
			return
		}
		if r, ok := m.(*metrics.Metrics); ok {
			r.MustRegister(cacheSize)
			r.MustRegister(cacheServices)
			r.MustRegister(cacheHits)
			r.MustRegister(cacheMisses)
			r.MustRegister(cachePrefetches)
		}
	})
	return nil
}