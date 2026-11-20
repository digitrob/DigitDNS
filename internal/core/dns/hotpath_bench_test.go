package dns

import (
	"net"
	"testing"

	"digitdns/internal/core/matcher"
	"digitdns/internal/database"

	"github.com/miekg/dns"
)

// These benchmarks pin the two per-query costs changed in this milestone:
// deriving the client IP from the transport address and rendering the answer
// section for the query log. Both sit on the cache-hit path, where upstream
// latency is already zero and allocations dominate the response time.

type benchmarkTelemetry struct {
	subscribers int64
	sink        []database.QueryLogItem
}

func (*benchmarkTelemetry) RecordQuery() {}
func (t *benchmarkTelemetry) PushQueryLog(item database.QueryLogItem) {
	t.sink = append(t.sink, item)
}
func (t *benchmarkTelemetry) SSESubscribers() int64 { return t.subscribers }

var benchmarkAnswers []database.DNSAnswer

func BenchmarkPeerIPUDP(b *testing.B) {
	addr := &net.UDPAddr{IP: net.ParseIP("198.51.100.42"), Port: 51234}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := peerIP(addr); got != "198.51.100.42" {
			b.Fatalf("peerIP = %q", got)
		}
	}
}

func BenchmarkPeerIPStringFallback(b *testing.B) {
	addr := net.UnixAddr{Name: "/tmp/dns.sock", Net: "unix"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = peerIP(&addr)
	}
}

func BenchmarkLoggingAnswersIdle(b *testing.B) {
	msg := responseWithA("cache.example.")
	telemetry := &benchmarkTelemetry{}
	h := NewHandler(&dummyAccess{}, nil, matcher.NewMatcher(), nil, telemetry, "198.51.100.1")
	h.SetQueryLogSink(func(database.QueryLogItem) {}, func() bool { return false })

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkAnswers = h.loggingAnswers(msg)
	}
}

func BenchmarkLoggingAnswersLive(b *testing.B) {
	msg := responseWithA("cache.example.")
	telemetry := &benchmarkTelemetry{subscribers: 1}
	h := NewHandler(&dummyAccess{}, nil, matcher.NewMatcher(), nil, telemetry, "198.51.100.1")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkAnswers = h.loggingAnswers(msg)
	}
}

func BenchmarkExtractAnswers(b *testing.B) {
	msg := new(dns.Msg)
	msg.SetQuestion("cache.example.", dns.TypeA)
	for i := 0; i < 8; i++ {
		rr, err := dns.NewRR("cache.example. 300 IN A 198.51.100.7")
		if err != nil {
			b.Fatal(err)
		}
		msg.Answer = append(msg.Answer, rr)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		benchmarkAnswers = extractAnswers(msg)
	}
}
