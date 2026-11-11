package upstream

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// Adding one upstream must not leave it unmeasured until the next full
// benchmark, and it must not touch the other resolvers in the pool.
func TestBenchmarkUpstreamMeasuresOnlyTheTarget(t *testing.T) {
	target := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10"}), nil)
	other := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.11"}), nil)
	p := newTestPool(t, []string{target.addr, other.addr}, time.Second, false, "")

	p.BenchmarkUpstream(target.addr)

	got := statFor(t, p, target.addr)
	if !got.Measured || got.LastProbeMs <= 0 || !got.LastProbeOK {
		t.Fatalf("newly added upstream was not measured: %+v", got)
	}
	if otherHits := other.udpHits.Load() + other.tcpHits.Load(); otherHits != 0 {
		t.Fatalf("benchmarking %s also probed %s (%d requests)", target.addr, other.addr, otherHits)
	}
}

// The cold first query includes the transport handshake and must not become the
// reported latency. Three warm probes follow it; the median should be fast even
// when the warm-up was deliberately slow.
func TestBenchmarkUpstreamDiscardsColdWarmup(t *testing.T) {
	var calls atomic.Int64
	resolver := newTestResolver(t, func(string) answer {
		if calls.Add(1) == 1 {
			return answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10", delay: 80 * time.Millisecond}
		}
		return answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10"}
	}, nil)
	p := newTestPool(t, []string{resolver.addr}, time.Second, false, "")

	p.BenchmarkUpstream(resolver.addr)

	got := statFor(t, p, resolver.addr)
	if !got.Measured || !got.LastProbeOK {
		t.Fatalf("benchmark did not record a successful measurement: %+v", got)
	}
	if got.LastProbeMs > 20 {
		t.Fatalf("cold warm-up leaked into the reported latency: %.1f ms", got.LastProbeMs)
	}
}

// One slow probe among the measured samples must not dominate; the median wins.
func TestBenchmarkUpstreamUsesMedianOfWarmSamples(t *testing.T) {
	var calls atomic.Int64
	resolver := newTestResolver(t, func(string) answer {
		n := calls.Add(1)
		if n == 2 { // first measured sample after the warm-up
			return answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10", delay: 80 * time.Millisecond}
		}
		return answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10"}
	}, nil)
	p := newTestPool(t, []string{resolver.addr}, time.Second, false, "")

	p.BenchmarkUpstream(resolver.addr)

	got := statFor(t, p, resolver.addr)
	if got.LastProbeMs > 20 {
		t.Fatalf("one slow sample dominated the benchmark: %.1f ms", got.LastProbeMs)
	}
}

func TestGetUpstreamStatsPrefersProbeUntilEnoughLiveSamples(t *testing.T) {
	const addr = "192.0.2.1:53"
	p := newTestPool(t, []string{addr}, time.Second, false, "")
	p.recordStat(addr, 20*time.Millisecond, statSuccess, "", true)
	p.recordStat(addr, 1718*time.Millisecond, statSuccess, "", false)

	got := statFor(t, p, addr)
	if !got.LatencyIsProbe {
		t.Fatal("one slow live sample was allowed to replace the probe median")
	}
	if got.LatencyP50Ms > 30 {
		t.Fatalf("displayed latency = %.1f ms, want the probe median", got.LatencyP50Ms)
	}

	// Two more live samples make the live median trustworthy.
	p.recordStat(addr, 1700*time.Millisecond, statSuccess, "", false)
	p.recordStat(addr, 1720*time.Millisecond, statSuccess, "", false)
	got = statFor(t, p, addr)
	if got.LatencyIsProbe {
		t.Fatal("live median was not used after three live samples")
	}
	if got.LatencyP50Ms < 1000 {
		t.Fatalf("displayed latency = %.1f ms, want the live median after three samples", got.LatencyP50Ms)
	}
}

// A resolver removed between SetUpstreams and the asynchronous probe must not
// reappear in the stats snapshot.
func TestBenchmarkUpstreamIgnoresRemovedResolver(t *testing.T) {
	removed := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.10"}), nil)
	kept := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.11"}), nil)
	p := newTestPool(t, []string{removed.addr, kept.addr}, time.Second, false, "")
	p.SetUpstreams([]string{kept.addr})

	p.BenchmarkUpstream(removed.addr)

	for _, stat := range p.GetUpstreamStats() {
		if stat.Address == removed.addr {
			t.Fatalf("removed resolver came back in stats: %+v", stat)
		}
	}
	if hits := removed.udpHits.Load() + removed.tcpHits.Load(); hits != 0 {
		t.Fatalf("removed resolver was probed %d times", hits)
	}
}
