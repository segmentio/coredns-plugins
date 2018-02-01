package coredns_consul

import (
	"context"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
)

// Consul is the implementation of the "consul" CoreDNS plugin.
//
// Consul instances are safe to use concurrently from multiple goroutines.
//
// Consul instances must not be copied after the first time they are used,
// referencing them by pointer should be preferred to passing them by value.
type Consul struct {
	Next plugin.Handler // Next handler in the list of plugins.

	// Addr is the address of the consul agent used by this plugin, it must be
	// be in the scheme://host:port format.
	Addr string

	// Maximum age of cached service entries.
	TTL time.Duration

	// Maximum number of inflight requests per target.
	MaxRequests int

	// Configuration of the cache prefetcher.
	PrefetchAmount     int
	PrefetchPercentage int
	PrefetchDuration   time.Duration

	// HTTP transport used to send requests to consul.
	Transport http.RoundTripper

	once  sync.Once
	cache *cache
}

const (
	defaultAddr               = "http://localhost:8500"
	defaultTTL                = 1 * time.Minute
	defaultMaxRequests        = 8192
	defaultPrefetchAmount     = 2
	defaultPrefetchPercentage = 90
	defaultPrefetchDuration   = 1 * time.Minute
)

// Consul constructs a new instance of a consul plugin.
func New() *Consul {
	return &Consul{
		Addr:               defaultAddr,
		TTL:                defaultTTL,
		MaxRequests:        defaultMaxRequests,
		PrefetchAmount:     defaultPrefetchAmount,
		PrefetchPercentage: defaultPrefetchPercentage,
		PrefetchDuration:   defaultPrefetchDuration,
	}
}

// Name of the plugin, returns "consul".
func (*Consul) Name() string { return "consul" }

// ServeDNS satisfies the plugin.Handler interface.
func (c *Consul) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	c.once.Do(c.init)

	qname := r.Question[0].Name
	qtype := r.Question[0].Qtype

	name, tag, typ, dc, domain := splitName(qname)
	if len(name) == 0 {
		return dns.RcodeNameError, nil
	}
	if domain != "consul" {
		return dns.RcodeRefused, nil
	}
	if typ != "service" {
		return dns.RcodeNotImplemented, nil
	}

	key := key{
		name:  name,
		tag:   tag,
		dc:    dc,
		qtype: r.Question[0].Qtype,
	}

	switch key.qtype {
	case dns.TypeA, dns.TypeAAAA, dns.TypeANY:
	case dns.TypeSRV:
		key.qtype = dns.TypeANY
	default:
		return dns.RcodeNotImplemented, nil
	}

	res := make(chan response, 1)
	req := request{key: key, res: res}

	select {
	case c.cache.reqs <- req:
	case <-ctx.Done():
		return dns.RcodeServerFailure, ctx.Err()
	}

	var found response
	select {
	case found = <-res:
	case <-ctx.Done():
		return dns.RcodeServerFailure, ctx.Err()
	}
	if found.err != nil {
		return dns.RcodeServerFailure, found.err
	}
	if found.srv.addr == nil {
		return dns.RcodeNameError, nil
	}

	var answer dns.RR
	var extra dns.RR
	switch qtype {
	case dns.TypeA:
		answer = found.A(qname)
	case dns.TypeAAAA:
		answer = found.AAAA(qname)
	case dns.TypeANY:
		answer = found.ANY(qname)
	case dns.TypeSRV:
		answer = found.SRV(qname)
		extra = found.ANY(answer.(*dns.SRV).Target)
	}

	r.Compress = true
	r.Authoritative = true
	r.Answer = append(r.Answer, answer)

	if extra != nil {
		r.Extra = append(r.Extra, extra)
	}

	w.WriteMsg(r)
	return dns.RcodeSuccess, nil
}

func (c *Consul) init() {
	reqs := make(chan request, c.MaxRequests)

	cache := &cache{
		reqs:               reqs,
		addr:               c.Addr,
		ttl:                c.TTL,
		maxRequests:        c.MaxRequests,
		prefetchAmount:     c.PrefetchAmount,
		prefetchPercentage: c.PrefetchPercentage,
		prefetchDuration:   c.PrefetchDuration,
		transport:          c.Transport,
	}

	if cache.transport == nil {
		cache.transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 10 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     2 * c.TTL,
		}
	}

	go cache.serve(reqs)

	c.cache = cache
	runtime.SetFinalizer(c, func(c *Consul) { c.cache.close() })
}

func splitName(s string) (name, tag, typ, dc, domain string) {
	s = strings.TrimSuffix(s, ".")
	if strings.HasPrefix(s, "_") {
		return splitNameRFC2782(s)
	}
	return splitNameDefault(s)
}

func splitNameDefault(s string) (name, tag, typ, dc, domain string) {
	for _, sep := range []string{".service.", ".query."} {
		if i := strings.Index(s, sep); i >= 0 {
			name, tag = splitLast(s[:i])
			domain, dc = splitLast(s[i+len(sep):])
			typ = sep
			typ = strings.TrimPrefix(typ, ".")
			typ = strings.TrimSuffix(typ, ".")
			break
		}
	}
	return
}

func splitNameRFC2782(s string) (name, tag, typ, dc, domain string) {
	name, s = split(s)
	tag, s = split(s)

	if domain, s = split(s); domain == "service" {
		if domain, s = split(s); len(s) != 0 {
			dc = domain
			if domain, s = split(s); len(s) != 0 {
				name = ""
				return
			}
		}
	}

	if tag == "_tcp" {
		tag = ""
	} else if !strings.HasPrefix(tag, "_") {
		name = ""
		return
	}

	name = strings.TrimPrefix(name, "_")
	tag = strings.TrimPrefix(tag, "_")
	typ = "service"
	return
}

func split(s string) (token, remain string) {
	if i := strings.IndexByte(s, '.'); i < 0 {
		token = s
	} else {
		token, remain = s[:i], s[i+1:]
	}
	return
}

func splitLast(s string) (token, remain string) {
	if i := strings.LastIndexByte(s, '.'); i < 0 {
		token = s
	} else {
		token, remain = s[i+1:], s[:i]
	}
	return
}
