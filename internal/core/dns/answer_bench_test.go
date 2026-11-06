package dns

import (
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
)

// The proxy, blocked, custom-record and self-name paths used to build an answer
// by formatting DNS text and parsing it with dns.NewRR. These two benchmarks keep
// that comparison honest without putting the parser back on the production path.
func BenchmarkMakeARecord(b *testing.B) {
	ip := net.ParseIP("203.0.113.10")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = makeARecord("play.example.com.", 60, ip)
	}
}

func BenchmarkTextARecord(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr, err := dns.NewRR(fmt.Sprintf("%s 60 IN A %s", "play.example.com.", "203.0.113.10"))
		if err != nil {
			b.Fatal(err)
		}
		_ = rr
	}
}
