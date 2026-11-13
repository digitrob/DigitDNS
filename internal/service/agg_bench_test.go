package service

import (
	"path/filepath"
	"testing"

	"digitdns/internal/aggregate"
	"digitdns/internal/core/cache"
	coredns "digitdns/internal/core/dns"
	"digitdns/internal/core/matcher"

	"github.com/miekg/dns"
	"net"
)

// BenchmarkProcessCacheHitWithAggregator is the cache-hit path with the 90-day
// aggregate store attached, so RecordTotal/RecordDetail and their locks are on
// the hot path. Compare against BenchmarkProcessCacheHit (no aggregator) to
// see what the aggregate store costs per query.
func BenchmarkProcessCacheHitWithAggregator(b *testing.B) {
	m := matcher.NewMatcher()
	c := cache.NewCache(1024, 60, 3600)
	stats := NewStatsService(nil, nil, nil)
	b.Cleanup(stats.Close)
	b.Cleanup(c.Close)
	agg, err := aggregate.Open(filepath.Join(b.TempDir(), "stats.db"), aggregate.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(agg.Close)
	stats.SetAggregator(agg)

	h := coredns.NewHandler(benchAccess{}, c, m, nil, stats, "203.0.113.10")
	q := new(dns.Msg)
	q.SetQuestion("cache.example.", dns.TypeA)
	q.Response = true
	q.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "cache.example.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.IPv4(203, 0, 113, 20),
	}}
	c.Put(q.Question[0], q)

	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn("cache.example"), dns.TypeA)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if resp := h.ProcessQuery(req, "127.0.0.1", "UDP"); resp == nil {
			b.Fatal("nil response")
		}
	}
}
