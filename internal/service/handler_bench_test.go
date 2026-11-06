package service

import (
	"net"
	"testing"

	"digitdns/internal/core/cache"
	coredns "digitdns/internal/core/dns"
	"digitdns/internal/core/matcher"
	"digitdns/internal/database"

	"github.com/miekg/dns"
)

type benchAccess struct{}

func (benchAccess) IsIPAllowed(string) (*database.Client, bool) { return nil, true }
func (benchAccess) IsAllowAll() bool                            { return true }

func benchmarkHandler(b *testing.B, proxied, blocked []string, warmCache bool) *coredns.Handler {
	b.Helper()
	m := matcher.NewMatcher()
	m.SetCustomRules(proxied, blocked, nil, nil)
	c := cache.NewCache(1024, 60, 3600)
	stats := NewStatsService(nil, nil, nil)
	b.Cleanup(stats.Close)
	b.Cleanup(c.Close)
	h := coredns.NewHandler(benchAccess{}, c, m, nil, stats, "203.0.113.10")
	if warmCache {
		q := new(dns.Msg)
		q.SetQuestion("cache.example.", dns.TypeA)
		q.Response = true
		q.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: "cache.example.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
			A:   net.IPv4(203, 0, 113, 20),
		}}
		c.Put(q.Question[0], q)
	}
	return h
}

func benchmarkQuery(b *testing.B, h *coredns.Handler, name string) {
	b.Helper()
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), dns.TypeA)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if resp := h.ProcessQuery(req, "127.0.0.1", "UDP"); resp == nil {
			b.Fatal("nil response")
		}
	}
}

func BenchmarkProcessProxy(b *testing.B) {
	h := benchmarkHandler(b, []string{"bench-proxy.example.com"}, nil, false)
	benchmarkQuery(b, h, "bench-proxy.example.com")
}

func BenchmarkProcessBlocked(b *testing.B) {
	h := benchmarkHandler(b, nil, []string{"bench-block.example.com"}, false)
	benchmarkQuery(b, h, "bench-block.example.com")
}

func BenchmarkProcessCacheHit(b *testing.B) {
	h := benchmarkHandler(b, nil, nil, true)
	benchmarkQuery(b, h, "cache.example")
}
