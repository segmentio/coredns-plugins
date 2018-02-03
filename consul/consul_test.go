package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	corednstest "github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

func TestConsul(t *testing.T) {
	services := []consulServerService{
		// host 1
		{node: "host-1.local.domain.", name: "service-1", addr: "192.168.0.1", port: 10001, pass: true, tags: []string{"zone-1"}},
		{node: "host-1.local.domain.", name: "service-2", addr: "192.168.0.1", port: 10002, pass: true, tags: []string{"zone-1"}},
		{node: "host-1.local.domain.", name: "service-3", addr: "192.168.0.1", port: 10003, pass: true, tags: []string{"zone-1"}},
		{node: "host-1.local.domain.", name: "service-1", addr: "192.168.0.1", port: 10004, pass: true, tags: []string{"zone-1"}},

		// host 2
		{node: "host-2.local.domain.", name: "service-1", addr: "192.168.0.2", port: 10011, pass: true, tags: []string{"zone-2"}},
		{node: "host-2.local.domain.", name: "service-2", addr: "192.168.0.2", port: 10012, pass: true, tags: []string{"zone-2"}},

		// host 3
		{node: "host-3.local.domain.", name: "service-1", addr: "2001:db8:85a3::8a2e:370:7334", port: 10021, pass: true, tags: []string{"zone-1"}},
	}

	tests := []struct {
		scenario string
		qname    string
		qtype    uint16
		rcode    int
		replies  []*dns.Msg
	}{
		{
			scenario: "sending a A query for a service returns the correct addresses",
			qname:    "service-1.service.consul.",
			qtype:    dns.TypeA,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrA("service-1.service.consul.", "192.168.0.1")}},
				{Answer: []dns.RR{rrA("service-1.service.consul.", "192.168.0.2")}},
			},
		},

		{
			scenario: "sending a AAAA query for a service returns the correct addresses",
			qname:    "service-1.service.consul.",
			qtype:    dns.TypeAAAA,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrAAAA("service-1.service.consul.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a ANY query for a service returns the correct addresses",
			qname:    "service-1.service.consul.",
			qtype:    dns.TypeANY,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrA("service-1.service.consul.", "192.168.0.1")}},
				{Answer: []dns.RR{rrA("service-1.service.consul.", "192.168.0.2")}},
				{Answer: []dns.RR{rrAAAA("service-1.service.consul.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a SRV query for a service returns the correct addresses and ports",
			qname:    "service-1.service.consul.",
			qtype:    dns.TypeSRV,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrSRV("service-1.service.consul.", "host-1.local.domain.", 10001)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("service-1.service.consul.", "host-1.local.domain.", 10004)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("service-1.service.consul.", "host-2.local.domain.", 10011)}, Extra: []dns.RR{rrA("host-2.local.domain.", "192.168.0.2")}},
				{Answer: []dns.RR{rrSRV("service-1.service.consul.", "host-3.local.domain.", 10021)}, Extra: []dns.RR{rrAAAA("host-3.local.domain.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a SRV query in RFC 2782 format for a service returns the correct addresses and ports",
			qname:    "_service-1._tcp.service.consul.",
			qtype:    dns.TypeSRV,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.consul.", "host-1.local.domain.", 10001)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.consul.", "host-1.local.domain.", 10004)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.consul.", "host-2.local.domain.", 10011)}, Extra: []dns.RR{rrA("host-2.local.domain.", "192.168.0.2")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.consul.", "host-3.local.domain.", 10021)}, Extra: []dns.RR{rrAAAA("host-3.local.domain.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a SRV query with a tag filters the result set to matching services",
			qname:    "zone-1.service-1.service.consul.",
			qtype:    dns.TypeSRV,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrSRV("zone-1.service-1.service.consul.", "host-1.local.domain.", 10001)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("zone-1.service-1.service.consul.", "host-1.local.domain.", 10004)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("zone-1.service-1.service.consul.", "host-3.local.domain.", 10021)}, Extra: []dns.RR{rrAAAA("host-3.local.domain.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a SRV query in RFC 2782 format for a service returns the correct addresses and ports",
			qname:    "_service-1._zone-1.service.consul.",
			qtype:    dns.TypeSRV,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrSRV("_service-1._zone-1.service.consul.", "host-1.local.domain.", 10001)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._zone-1.service.consul.", "host-1.local.domain.", 10004)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._zone-1.service.consul.", "host-3.local.domain.", 10021)}, Extra: []dns.RR{rrAAAA("host-3.local.domain.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a A query for a service name that does not exist returns a NXDOMAIN error",
			qname:    "whatever.service.consul.",
			qtype:    dns.TypeA,
			rcode:    dns.RcodeNameError,
		},

		{
			scenario: "sending a A query for a consul prepared query returns a NOTIMPL error",
			qname:    "service-1.query.consul.",
			qtype:    dns.TypeA,
			rcode:    dns.RcodeNotImplemented,
		},

		{
			scenario: "sending a A query for a domain that is not consul. returns a REFUSED error",
			qname:    "service-1.service.other.",
			qtype:    dns.TypeA,
			rcode:    dns.RcodeRefused,
		},

		{
			scenario: "sending a A query missing the service name returns a NXDOMAIN error",
			qname:    ".service.consul.",
			qtype:    dns.TypeA,
			rcode:    dns.RcodeNameError,
		},

		{
			scenario: "sending a A query for a datacenter of the server returns the correct addreses",
			qname:    "service-1.service.dc1.consul.",
			qtype:    dns.TypeA,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrA("service-1.service.dc1.consul.", "192.168.0.1")}},
				{Answer: []dns.RR{rrA("service-1.service.dc1.consul.", "192.168.0.2")}},
			},
		},

		{
			scenario: "sending a A query for a datacenter that the server does not know about returns a NXDOMAIN error",
			qname:    "service-1.service.dc2.consul.",
			qtype:    dns.TypeA,
			rcode:    dns.RcodeNameError,
		},

		{
			scenario: "sending a SRV query in RFC 2782 format for a datacenter of the server returns the correct addresses and ports",
			qname:    "_service-1._tcp.service.dc1.consul.",
			qtype:    dns.TypeSRV,
			replies: []*dns.Msg{
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.dc1.consul.", "host-1.local.domain.", 10001)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.dc1.consul.", "host-1.local.domain.", 10004)}, Extra: []dns.RR{rrA("host-1.local.domain.", "192.168.0.1")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.dc1.consul.", "host-2.local.domain.", 10011)}, Extra: []dns.RR{rrA("host-2.local.domain.", "192.168.0.2")}},
				{Answer: []dns.RR{rrSRV("_service-1._tcp.service.dc1.consul.", "host-3.local.domain.", 10021)}, Extra: []dns.RR{rrAAAA("host-3.local.domain.", "2001:db8:85a3::8a2e:370:7334")}},
			},
		},

		{
			scenario: "sending a SRV query in RFC 2782 format for a datacenter of that the server does not know about returns a NSDOMAIN error",
			qname:    "_service-1._tcp.service.dc2.consul.",
			qtype:    dns.TypeSRV,
			rcode:    dns.RcodeNameError,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.scenario, func(t *testing.T) {
			t.Parallel()

			server := consulServer("dc1", services)
			defer server.Close()

			consul := New()
			consul.Addr = server.URL

			for i := 0; i != 10; i++ {
				req := &dns.Msg{}
				req.SetQuestion(dns.Fqdn(test.qname), test.qtype)
				rec := dnstest.NewRecorder(&corednstest.ResponseWriter{})

				rcode, err := consul.ServeDNS(context.Background(), rec, req)
				if err != nil {
					t.Errorf("Error: %v", err)
					return
				}
				if rcode != test.rcode {
					t.Errorf("Expected return code %v but got %v", test.rcode, rcode)
					return
				}

				if len(test.replies) != 0 {
					found := false
					for _, reply := range test.replies {
						if found = replyEqual(reply, rec.Msg); found {
							break
						}
					}
					if !found {
						t.Errorf("Unexpected reply: %v", rec.Msg)
					}
				}
			}
		})
	}
}

func consulServer(serverDC string, serverServices []consulServerService) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/v1/health/service/"

		if !strings.HasPrefix(r.URL.Path, prefix) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var (
			service = strings.TrimPrefix(r.URL.Path, prefix)
			query   = r.URL.Query()
			tag     = query.Get("tag")
			dc      = query.Get("dc")
			pass    = query.Get("passing")
			results = make([]consulHealthService, 0, len(serverServices))
		)

		if len(dc) == 0 || dc == serverDC {
			for _, srv := range serverServices {
				if srv.name != service {
					continue
				}
				if len(tag) != 0 && !srv.hasTag(tag) {
					continue
				}
				if len(pass) != 0 && !srv.pass {
					continue
				}
				results = append(results, consulHealthService{
					Node:    consulNode{Node: srv.node},
					Service: consulService{Address: srv.addr, Port: srv.port},
				})
			}
		}

		json.NewEncoder(w).Encode(results)
	}))
}

type consulServerService struct {
	node string
	name string
	addr string
	port int
	pass bool
	tags []string
}

func (srv *consulServerService) hasTag(tag string) bool {
	for _, srvTag := range srv.tags {
		if srvTag == tag {
			return true
		}
	}
	return false
}

func replyEqual(r1, r2 *dns.Msg) bool {
	return rrEqual(r1.Answer, r2.Answer) && rrEqual(r1.Ns, r2.Ns) && rrEqual(r1.Extra, r2.Extra)
}

func rrEqual(rr1, rr2 []dns.RR) bool {
	if len(rr1) != len(rr2) {
		return false
	}
	for i := range rr1 {
		h1 := rr1[i].Header()
		h2 := rr2[i].Header()
		assertNonZeroTTL(h1)
		assertNonZeroTTL(h2)

		ttl1 := h1.Ttl
		ttl2 := h2.Ttl

		// TTLs are runtime dependent, don't compare.
		h1.Ttl = 0
		h2.Ttl = 0

		ok := reflect.DeepEqual(rr1[i], rr2[i])

		h1.Ttl = ttl1
		h2.Ttl = ttl2

		if !ok {
			return false
		}
	}
	return true
}

func assertNonZeroTTL(h *dns.RR_Header) {
	if h.Ttl == 0 {
		panic(fmt.Sprintf("TTL cannot be zero: %v", h))
	}
}

func rrA(name, addr string) *dns.A {
	return &dns.A{Hdr: rrHeader(name, dns.TypeA), A: net.ParseIP(addr)}
}

func rrAAAA(name, addr string) *dns.AAAA {
	return &dns.AAAA{Hdr: rrHeader(name, dns.TypeAAAA), AAAA: net.ParseIP(addr)}
}

func rrSRV(name string, target string, port int) *dns.SRV {
	return &dns.SRV{Hdr: rrHeader(name, dns.TypeSRV), Priority: 1, Weight: 1, Port: uint16(port), Target: target}
}

func rrHeader(name string, rrtype uint16) dns.RR_Header {
	return dns.RR_Header{Name: name, Rrtype: rrtype, Class: dns.ClassINET, Ttl: 1}
}
