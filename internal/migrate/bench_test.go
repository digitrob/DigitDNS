package migrate

import (
	"fmt"
	"strings"
	"testing"
)

// benchmarkBundle builds a bundle large enough to make the normalization and
// merge costs visible without turning `go test -bench` into a memory test.
func benchmarkBundle(domains int) Bundle {
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourcePiHoleV6, Version: "v6.0"},
		Proxied:       make([]string, 0, domains),
		Records:       make([]Record, 0, 1000),
		Clients:       make([]Client, 0, 500),
		Rewrites:      make([]Rewrite, 0, 500),
	}
	for i := 0; i < domains; i++ {
		bundle.Proxied = append(bundle.Proxied, fmt.Sprintf("host-%d.example", i))
	}
	for i := 0; i < 1000; i++ {
		bundle.Records = append(bundle.Records, Record{
			Name:  fmt.Sprintf("pin-%d.example", i),
			Type:  "A",
			Value: fmt.Sprintf("192.0.2.%d", i%250+1),
		})
	}
	for i := 0; i < 500; i++ {
		bundle.Clients = append(bundle.Clients, Client{
			Name: fmt.Sprintf("client-%d", i),
			IDs:  []string{fmt.Sprintf("198.51.100.%d", i%250+1)},
		})
	}
	for i := 0; i < 500; i++ {
		bundle.Rewrites = append(bundle.Rewrites, Rewrite{
			Domain: fmt.Sprintf("rewrite-%d.example", i),
			Target: fmt.Sprintf("203.0.113.%d", i%250+1),
		})
	}
	for i := 0; i < 20; i++ {
		var content strings.Builder
		for j := 0; j < 100; j++ {
			fmt.Fprintf(&content, "0.0.0.0 list-%d-%d.example\n", i, j)
		}
		bundle.FilterLists = append(bundle.FilterLists, FilterList{
			Name:    fmt.Sprintf("list-%d", i),
			Enabled: true,
			Content: content.String(),
		})
	}
	return bundle
}

func BenchmarkNormalizeBundle(b *testing.B) {
	bundle := benchmarkBundle(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bundle.Normalize()
	}
}

func BenchmarkMergeBundle(b *testing.B) {
	bundle := benchmarkBundle(10000)
	target := NewTarget()
	for i := 0; i < 5000; i++ {
		target.Proxied = append(target.Proxied, fmt.Sprintf("existing-%d.example", i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Merge(target, bundle)
	}
}

func BenchmarkCheckCustomRules(b *testing.B) {
	var rules strings.Builder
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&rules, "/^ad[0-9]+\\.example%d$/\n", i)
	}
	text := rules.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckCustomRules(text)
	}
}
