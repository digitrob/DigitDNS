package blocklist

import (
	"fmt"
	"strings"
	"testing"
)

func benchmarkList(domains int) string {
	var text strings.Builder
	text.Grow(domains * 24)
	for i := 0; i < domains; i++ {
		fmt.Fprintf(&text, "0.0.0.0 ads-%d.example\n", i)
	}
	return text.String()
}

func BenchmarkBuildLargeList(b *testing.B) {
	text := benchmarkList(50000)
	sources := []Source{{Name: "bench", Text: text, Enabled: true}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Build(sources, nil)
	}
}

func BenchmarkAnalyzeLargeList(b *testing.B) {
	text := benchmarkList(50000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Analyze(text)
	}
}

func BenchmarkMatchLargeList(b *testing.B) {
	text := benchmarkList(50000)
	idx, _ := Build([]Source{{Name: "bench", Text: text, Enabled: true}}, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = idx.Match("ads-49999.example")
	}
}
