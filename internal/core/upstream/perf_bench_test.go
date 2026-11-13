package upstream

import (
	"testing"
	"time"
)

// BenchmarkOrderedByLatencyWarm measures the cached lookup on the hot path. The
// first run populates the snapshot; every later iteration must be a single atomic
// load plus one slice compare, with no lock and no map walk.
func BenchmarkOrderedByLatencyWarm(b *testing.B) {
	p := NewUpstreamPool([]string{"192.0.2.1:53", "192.0.2.2:53"}, time.Second, false, "")
	addrs := []string{"192.0.2.1:53", "192.0.2.2:53"}
	if got := p.orderedByLatency(addrs); len(got) == 0 {
		b.Fatal("orderedByLatency returned empty")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := p.orderedByLatency(addrs); len(got) == 0 {
			b.Fatal("orderedByLatency returned empty")
		}
	}
}

// BenchmarkRecordStat measures the per-query stat recording after the atomic
// stats view landed. Before the view it took a pool RLock plus a per-stat Lock on
// every live upstream answer.
func BenchmarkRecordStat(b *testing.B) {
	p := NewUpstreamPool([]string{"192.0.2.1:53"}, time.Second, false, "")
	addr := "192.0.2.1:53"
	if _, ok := p.stats[addr]; !ok {
		p.stats[addr] = &upstreamStatInternal{lastUpdated: time.Now()}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.recordStat(addr, time.Millisecond, statSuccess, "", false)
	}
}

// BenchmarkGetUpstreamStats is the stats fan-out read path, kept here so the
// per-query hot paths and the once-a-second read path stay measured together.
func BenchmarkGetUpstreamStats(b *testing.B) {
	p := NewUpstreamPool([]string{"192.0.2.1:53", "192.0.2.2:53"}, time.Second, false, "")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.GetUpstreamStats()
	}
}
