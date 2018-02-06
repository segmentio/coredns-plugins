package consul

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/miekg/dns"
)

type cache struct {
	reqs               chan<- serviceRequest
	addr               string
	ttl                time.Duration
	maxRequests        int
	prefetchAmount     int
	prefetchPercentage int
	prefetchDuration   time.Duration
	transport          http.RoundTripper
}

func (c *cache) close() { close(c.reqs) }

func (c *cache) serve(reqs <-chan serviceRequest) {
	prefetches := make(map[key]*serviceCache)
	caches := make(map[key]*serviceCache)

	ready := make(chan *serviceCache, c.maxRequests)
	done := make(chan *serviceCache, c.maxRequests)
	next := make(chan key, c.maxRequests)

dispatchRequests:
	for {
		select {
		case sc := <-done:
			if caches[sc.key] == sc {
				delete(caches, sc.key)
				cacheEvictionsInc()

				if sc.negative() {
					log.Printf("[INFO] %s: negative cache expired", sc.key)
					cacheSizeAddDenial(-1)
				} else {
					log.Printf("[INFO] %s: cache of %d services expired", sc.key, len(sc.services))
					cacheSizeAddSuccess(-1)
					cacheServicesAdd(-len(sc.services))
				}
			}

		case key := <-next:
			if _, alreadyPrefetching := prefetches[key]; !alreadyPrefetching {
				prefetches[key] = c.spawn(key, ready, done, next)
				cachePrefetchesInc()
			}

		case sc := <-ready:
			if prefetches[sc.key] != sc {
				sc.close()
			} else {
				delete(prefetches, sc.key)
				expired := caches[sc.key]
				caches[sc.key] = sc

				if expired != nil {
					if expired.negative() {
						cacheSizeAddDenial(-1)
					} else {
						cacheSizeAddSuccess(-1)
						cacheServicesAdd(-len(expired.services))
					}
					expired.close()
				}

				if sc.negative() {
					cacheSizeAddDenial(1)
				} else {
					cacheSizeAddSuccess(1)
					cacheServicesAdd(len(sc.services))
					cacheFetchSizesObserve(len(sc.services))
				}
			}

		case r, ok := <-reqs:
			if !ok {
				break dispatchRequests
			}

			sc, hit := caches[r.key]
			if !hit {
				sc, hit = prefetches[r.key]
				if !hit {
					sc = c.spawn(r.key, ready, done, next)
					prefetches[r.key] = sc
					cacheMissesInc()
				}
			}

			select {
			case sc.reqs <- r:
			default:
				r.reject(errTooManyRequests)
			}
		}
	}

	for _, sc := range prefetches {
		sc.close()
	}

	for _, sc := range caches {
		sc.close()
	}

	for len(caches) != 0 || len(prefetches) != 0 {
		select {
		case <-next:
			// ignore

		case sc := <-done:
			if caches[sc.key] == sc {
				delete(caches, sc.key)
			}

		case sc := <-ready:
			if prefetches[sc.key] == sc {
				delete(prefetches, sc.key)
			}
		}
	}
}

func (c *cache) spawn(key key, ready chan<- *serviceCache, done chan<- *serviceCache, next chan<- key) *serviceCache {
	reqs := make(chan serviceRequest, c.maxRequests)

	sc := &serviceCache{
		reqs:               reqs,
		key:                key,
		addr:               c.addr,
		ttl:                c.ttl,
		prefetchAmount:     c.prefetchAmount,
		prefetchPercentage: c.prefetchPercentage,
		prefetchDuration:   c.prefetchDuration,
		transport:          c.transport,
		rand:               rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Add jitter to the cache, lasting at least half the configured TTL value.
	// This is intended to prevent synchronizing cache expirations when the
	// cache fills up at the boot of the DNS server.
	sc.ttl /= 2
	sc.ttl += time.Duration(sc.rand.Int63n(int64(sc.ttl)))

	go func() {
		t0 := time.Now()
		sc.services, sc.err = sc.load()
		t1 := time.Now()

		rtt := t1.Sub(t0)
		ttl := sc.ttl.Round(time.Second)

		if sc.err != nil {
			log.Printf("[INFO] %s: fetch completed in %s: caching error for %s (%s)", key, rtt, ttl, sc.err)
		} else {
			log.Printf("[INFO] %s: fetch completed in %s: caching %d service endpoints for %s", key, rtt, len(sc.services), ttl)
		}

		go sc.serve(reqs, done, next)
		ready <- sc
		cacheFetchDurationsObserve(rtt)
	}()

	return sc
}

type serviceCache struct {
	reqs               chan<- serviceRequest
	key                key
	err                error
	addr               string
	ttl                time.Duration
	prefetchAmount     int
	prefetchPercentage int
	prefetchDuration   time.Duration
	transport          http.RoundTripper
	rand               *rand.Rand
	services           []service
}

func (c *serviceCache) negative() bool { return c.err != nil }

func (c *serviceCache) close() { close(c.reqs) }

func (c *serviceCache) serve(reqs <-chan serviceRequest, done chan<- *serviceCache, next chan<- key) {
	const timeBuckets = 4
	deadline := time.Now().Add(c.ttl)

	timeout := time.NewTimer(c.ttl)
	defer timeout.Stop()

	prefetchCheck := time.NewTicker(c.prefetchDuration / timeBuckets)
	defer prefetchCheck.Stop()

	prefetchTrigger := time.NewTimer(c.ttl - ((c.ttl * time.Duration(c.prefetchPercentage)) / 100))
	defer prefetchTrigger.Stop()

	hits := 0
	prefetch := false
	roundRobin := 0

	get := func() (srv service) {
		if len(c.services) != 0 {
			index := roundRobin
			roundRobin = (roundRobin + 1) % len(c.services)
			srv = c.services[index]
		}
		return
	}

	ttl := func() time.Duration {
		d := deadline.Sub(time.Now())
		if d < 0 {
			d = 0
		}
		return d
	}

	respond := func(req serviceRequest) {
		if c.err != nil {
			req.reject(c.err)
			cacheHitsIncDenial()
		} else {
			req.resolve(get(), ttl())
			cacheHitsIncSuccess()
		}
	}

	for {
		select {
		case <-timeout.C:
			done <- c
			for req := range reqs {
				respond(req)
			}
			return

		case <-prefetchCheck.C:
			if !prefetch {
				prefetch = hits >= c.prefetchAmount
				hits -= (hits / timeBuckets) // assumes distribution over time
			}

		case <-prefetchTrigger.C:
			if prefetch {
				select {
				case next <- c.key:
				default:
					// If we failed schedule the prefetch it means the channel
					// was full and likely the program is overloaded. Just give
					// up and leave the cache expire, it will get refetched on
					// the next query for this service name.
				}
			}

		case req, ok := <-reqs:
			if !ok {
				return
			}
			respond(req)
			hits++
		}
	}
}

func (c *serviceCache) load() ([]service, error) {
	u := c.addr + "/v1/health/service/" + url.QueryEscape(c.key.name) + "?passing"
	if len(c.key.tag) != 0 {
		u += "&tag=" + url.QueryEscape(c.key.tag)
	}
	if len(c.key.dc) != 0 {
		u += "&dc=" + url.QueryEscape(c.key.dc)
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
		return nil, fmt.Errorf("GET %s: %s", u, res.Status)
	}

	var endpoints = make([]consulHealthService, 0, 100)
	if err := json.NewDecoder(res.Body).Decode(&endpoints); err != nil {
		return nil, err
	}
	if err := res.Body.Close(); err != nil {
		return nil, err
	}

	var isOK = isIP
	switch c.key.qtype {
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
				node: dns.Fqdn(endpoint.Node.Node),
			})
		}
	}
	for i, j := range c.rand.Perm(len(services)) {
		services[i], services[j] = services[j], services[i]
	}
	return services, nil
}

type key struct {
	name  string
	tag   string
	dc    string
	qtype uint16
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

type serviceRequest struct {
	key key
	res chan<- serviceResponse
}

func (req serviceRequest) resolve(srv service, ttl time.Duration) {
	req.res <- serviceResponse{srv: srv, ttl: ttl}
}
func (req serviceRequest) reject(err error) { req.res <- serviceResponse{err: err} }

type serviceResponse struct {
	srv service
	err error
	ttl time.Duration
}

func (r serviceResponse) header(name string, rrtype uint16) dns.RR_Header {
	return dns.RR_Header{
		Name:   name,
		Rrtype: rrtype,
		Class:  dns.ClassINET,
		Ttl:    uint32(r.ttl.Round(time.Second)),
	}
}

func (r serviceResponse) A(name string) *dns.A {
	return &dns.A{
		Hdr: r.header(name, dns.TypeA),
		A:   r.srv.addr,
	}
}

func (r serviceResponse) AAAA(name string) *dns.AAAA {
	return &dns.AAAA{
		Hdr:  r.header(name, dns.TypeAAAA),
		AAAA: r.srv.addr,
	}
}

func (r serviceResponse) SRV(name string) *dns.SRV {
	return &dns.SRV{
		Hdr:      r.header(name, dns.TypeSRV),
		Priority: 1,
		Weight:   1,
		Port:     uint16(r.srv.port),
		Target:   r.srv.node,
	}
}

func (r serviceResponse) ANY(name string) dns.RR {
	if isIPv6(r.srv.addr) {
		return r.AAAA(name)
	}
	return r.A(name)
}

// https://www.consul.io/api/health.html#list-nodes-for-service
type consulHealthService struct {
	Node    consulNode
	Service consulService
}

type consulNode struct {
	Node string
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
