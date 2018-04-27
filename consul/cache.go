package consul

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

type cache struct {
	addr               string
	ttl                time.Duration
	prefetchAmount     int
	prefetchPercentage int
	prefetchDuration   time.Duration
	transport          http.RoundTripper

	mutex    sync.RWMutex
	entries  map[key]*entry
	lookups  atomicIndex
	cleanups atomicLock
}

func (c *cache) prefetchDeadlineOf(e *entry) time.Time {
	d := float64(c.ttl) * (float64(c.prefetchPercentage) / 1000)
	return e.exp.Add(-time.Duration(d))
}

func (c *cache) expirationTimeFrom(now time.Time) time.Time {
	return now.Add(c.ttl + time.Duration(rand.Int63n(int64(c.ttl/2))))
}

func (c *cache) lookup(ctx context.Context, k key, now time.Time) (srv service, ttl time.Duration, err error) {
	hit := true
	m := k.metrics()
	e := c.grab(k, now)
	i := e.index.incr() - 1

	// Note: the implementation of this check should be changed to take prefetchAmount
	// into account, but it requires maintaining more state to implement it right, which
	// is not immediately needed since consul service names looked up in production are
	// all very popular.
	if i == 0 || (i >= uint32(c.prefetchAmount)) && now.After(c.prefetchDeadlineOf(e)) {
		if e.lock.tryLock() {
			t0 := time.Now()
			srv, err := c.load(k)
			t1 := time.Now()
			e.lock.unlock()

			if e.once.tryLock() {
				e.srv = srv
				e.err = err
				close(e.ready)

				if err == nil {
					m.cacheSizeAddSuccess(1)
				} else {
					m.cacheSizeAddDenial(1)
				}

				hit = false
				m.cacheMissesInc()
				m.cacheServicesAdd(len(srv))

			} else if err == nil {
				c.update(k, &entry{
					srv:   srv,
					exp:   c.expirationTimeFrom(now),
					ready: e.ready, // already closed
					index: 1,       // can't be zero to avoid refetching on next lookup
					once:  1,       // can't be zero to avoid closing the channel twice
				})
				m.cachePrefetchesInc()
			}

			m.cacheFetchSizesObserve(len(srv))
			m.cacheFetchDurationsObserve(t1.Sub(t0))
		}
	}

	// Every 1000 lookups the cache removes expired entries.
	if (c.lookups.incr() % 1000) == 0 {
		if c.cleanups.tryLock() {
			c.cleanup(now)
			c.cleanups.unlock()
		}
	}

	if !e.isReady() {
		select {
		case <-e.ready:
		case <-ctx.Done():
			err = ctx.Err()
			return
		}
	}

	if n := len(e.srv); n != 0 {
		srv = e.srv[i%uint32(n)]
	}

	ttl = e.exp.Sub(now)
	err = e.err

	if hit {
		if err == nil {
			m.cacheHitsIncSuccess()
		} else {
			m.cacheHitsIncDenial()
		}
	}

	return
}

func (c *cache) grab(k key, now time.Time) (e *entry) {
	c.mutex.RLock()
	e = c.entries[k]
	c.mutex.RUnlock()

	if e == nil {
		c.mutex.Lock()

		if e = c.entries[k]; e == nil {
			if c.entries == nil {
				c.entries = make(map[key]*entry)
			}

			e = &entry{
				exp:   c.expirationTimeFrom(now),
				ready: make(chan struct{}),
			}

			c.entries[k] = e
		}

		c.mutex.Unlock()
	}

	return e
}

func (c *cache) update(k key, e *entry) {
	c.mutex.Lock()
	c.entries[k] = e
	c.mutex.Unlock()
}

func (c *cache) load(k key) ([]service, error) {
	u := c.addr + "/v1/health/service/" + url.QueryEscape(k.name) + "?passing"
	if len(k.tag) != 0 {
		u += "&tag=" + url.QueryEscape(k.tag)
	}
	if len(k.dc) != 0 {
		u += "&dc=" + url.QueryEscape(k.dc)
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.ttl)
	defer cancel()

	res, err := c.transport.RoundTrip(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, httpError(res)
	}

	var endpoints = make([]consulHealthService, 0, 100)
	if err := json.NewDecoder(res.Body).Decode(&endpoints); err != nil {
		return nil, err
	}
	if err := res.Body.Close(); err != nil {
		return nil, err
	}

	var isOK = isIP
	switch k.qtype {
	case dns.TypeA:
		isOK = isIPv4
	case dns.TypeAAAA:
		isOK = isIPv6
	}

	var services = make([]service, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if ip := net.ParseIP(endpoint.Service.Address); isOK(ip) {
			services = append(services, service{
				addr: ip,
				port: endpoint.Service.Port,
				node: dns.Fqdn(join(endpoint.Node.Node, "node", endpoint.Node.Datacenter, "consul")),
			})
		}
	}
	for i := range services {
		j := rand.Intn(len(services))
		services[i], services[j] = services[j], services[i]
	}
	return services, nil
}

// cleanup removes all expired cache entries. The implementation optimizes for
// creating opportunities for other goroutines to get scheduled by frequently
// releasing and reacquiring locks on the cache mutex.
func (c *cache) cleanup(now time.Time) {
	c.mutex.RLock()

	for k, e := range c.entries {
		c.mutex.RUnlock()

		if now.After(e.exp) && e.isReady() {
			removed := false

			c.mutex.Lock()
			// In case the entries map was modified concurrently, we make
			// sure that the e we've seen is still the one in the cache.
			if c.entries[k] == e {
				delete(c.entries, k)
				removed = true
			}
			c.mutex.Unlock()

			if removed {
				m := k.metrics()
				if e.err == nil {
					m.cacheSizeAddDenial(-1)
				} else {
					m.cacheSizeAddSuccess(-1)
				}
				m.cacheServicesAdd(-len(e.srv))
			}
		}

		c.mutex.RLock()
	}

	c.mutex.RUnlock()
}

func httpError(res *http.Response) error {
	req := res.Request
	return fmt.Errorf("%s %s: %s", req.Method, req.URL, res.Status)
}

func join(parts ...string) string {
	b := make([]byte, 0, 10*len(parts))

	for _, s := range parts {
		if len(s) != 0 {
			if len(b) != 0 {
				b = append(b, '.')
			}
			b = append(b, s...)
		}
	}

	return string(b)
}

type key struct {
	name  string
	tag   string
	dc    string
	qtype uint16
}

func (k key) metrics() metrics {
	return metrics{name: k.name, tag: k.tag, dc: k.dc}
}

func (k key) String() string {
	b := make([]byte, 0, 100)
	b = append(b, dns.TypeToString[k.qtype]...)
	b = append(b, ' ')

	if len(k.tag) != 0 {
		b = append(b, k.tag...)
		b = append(b, '.')
	}

	b = append(b, k.name...)
	b = append(b, ".service"...)

	if len(k.dc) != 0 {
		b = append(b, '.')
		b = append(b, k.dc...)
	}

	b = append(b, ".consul"...)
	return string(b)
}

type service struct {
	addr net.IP
	port int
	node string
}

func (s service) header(name string, rrtype uint16, ttl time.Duration) dns.RR_Header {
	return dns.RR_Header{
		Name:   name,
		Rrtype: rrtype,
		Class:  dns.ClassINET,
		Ttl:    1 + uint32(ttl.Truncate(time.Second)/time.Second),
	}
}

func (s service) A(name string, ttl time.Duration) *dns.A {
	return &dns.A{
		Hdr: s.header(name, dns.TypeA, ttl),
		A:   s.addr,
	}
}

func (s service) AAAA(name string, ttl time.Duration) *dns.AAAA {
	return &dns.AAAA{
		Hdr:  s.header(name, dns.TypeAAAA, ttl),
		AAAA: s.addr,
	}
}

func (s service) SRV(name string, ttl time.Duration) *dns.SRV {
	return &dns.SRV{
		Hdr:      s.header(name, dns.TypeSRV, ttl),
		Priority: 1,
		Weight:   1,
		Port:     uint16(s.port),
		Target:   s.node,
	}
}

func (s service) ANY(name string, ttl time.Duration) dns.RR {
	if isIPv6(s.addr) {
		return s.AAAA(name, ttl)
	}
	return s.A(name, ttl)
}

type entry struct {
	srv   []service
	err   error
	exp   time.Time
	ready chan struct{}
	index atomicIndex
	lock  atomicLock
	once  atomicLock
}

func (e *entry) isReady() bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

// https://www.consul.io/api/health.html#list-nodes-for-service
type consulHealthService struct {
	Node    consulNode
	Service consulService
}

type consulNode struct {
	Node       string
	Datacenter string
}

type consulService struct {
	Address string
	Port    int
}

var (
	errTooManyRequests = errors.New("too many requests")
)

func isIP(ip net.IP) bool   { return ip != nil }
func isIPv4(ip net.IP) bool { return ip != nil && ip.To4() != nil }
func isIPv6(ip net.IP) bool { return ip != nil && ip.To4() == nil }

type atomicIndex uint32

func (index *atomicIndex) incr() uint32 {
	return atomic.AddUint32((*uint32)(index), 1)
}

type atomicLock uint32

func (lock *atomicLock) tryLock() bool {
	return atomic.CompareAndSwapUint32((*uint32)(lock), 0, 1)
}

func (lock *atomicLock) unlock() {
	atomic.StoreUint32((*uint32)(lock), 0)
}
