package resolution

import (
	"fmt"
	"testing"
)

// BenchmarkMatchLinear and BenchmarkProfileIndexMatch use the same 200-entry
// policy: the first is the old per-query scan, the second is the compiled index
// the upstream pool now installs. The domain is a deep subdomain so the linear
// path has to normalise many patterns before it finds the wildcard winner.
func BenchmarkMatchLinear(b *testing.B) {
	profiles := benchmarkProfiles()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := Match(profiles, "cdn.deep.api.example.com"); !ok {
			b.Fatal("no match")
		}
	}
}

func BenchmarkProfileIndexMatch(b *testing.B) {
	idx := CompileProfiles(benchmarkProfiles())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := idx.Match("cdn.deep.api.example.com"); !ok {
			b.Fatal("no match")
		}
	}
}

func benchmarkProfiles() map[string]Profile {
	profiles := make(map[string]Profile, 200)
	for i := 0; i < 200; i++ {
		profiles[fmt.Sprintf("*.service-%d.example.com", i)] = Profile{Upstream: "tls://dns.example:853"}
	}
	// The winner is the longest wildcard suffix for the benchmarked name.
	profiles["*.api.example.com"] = Profile{Strategy: StrategyHedged}
	return profiles
}
