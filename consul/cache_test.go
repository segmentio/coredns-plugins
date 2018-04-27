package consul

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkCache(b *testing.B) {
	handler := consulHandler("dc1", []consulServerService{
		// host 1
		{node: "host-1", name: "service-1", addr: "192.168.0.1", port: 10001, pass: true, tags: []string{"zone-1"}},
		{node: "host-1", name: "service-2", addr: "192.168.0.1", port: 10002, pass: true, tags: []string{"zone-1"}},
		{node: "host-1", name: "service-3", addr: "192.168.0.1", port: 10003, pass: true, tags: []string{"zone-1"}},
		{node: "host-1", name: "service-1", addr: "192.168.0.1", port: 10004, pass: true, tags: []string{"zone-1"}},

		// host 2
		{node: "host-2", name: "service-1", addr: "192.168.0.2", port: 10011, pass: true, tags: []string{"zone-2"}},
		{node: "host-2", name: "service-2", addr: "192.168.0.2", port: 10012, pass: true, tags: []string{"zone-2"}},

		// host 3
		{node: "host-3", name: "service-1", addr: "2001:db8:85a3::8a2e:370:7334", port: 10021, pass: true, tags: []string{"zone-1"}},
	})

	calls := int64(0)
	lookups := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	cache := cache{
		addr:               server.URL,
		ttl:                1 * time.Second,
		prefetchAmount:     10,
		prefetchPercentage: 10,
		prefetchDuration:   1 * time.Second,
		transport:          http.DefaultTransport,
	}

	keys := []key{
		{name: "service-1"},
		{name: "service-2"},
		{name: "service-3"},
		{name: "service-4"},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()

		for i := 0; pb.Next(); i++ {
			atomic.AddInt64(&lookups, 1)
			cache.lookup(ctx, keys[i%len(keys)], time.Now())
		}
	})

	b.Logf("lookups: %d, calls: %d, hit-rate: %.2f%%", lookups, calls, 100*(1-(float64(calls)/float64(lookups))))
}
