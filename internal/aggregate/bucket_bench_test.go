package aggregate

import (
	"testing"
	"time"

	"digitdns/internal/database"
)

// Bucket.record runs once per resolved query while the aggregator mutex is held,
// so its allocations are on the resolver's hot path rather than on a reporting
// path. The canonical label helpers keep the common action/protocol values
// allocation-free; run this with -benchmem before and after touching them.
func BenchmarkBucketRecord(b *testing.B) {
	bucket := newBucket()
	cfg := Config{TopDomains: 200, TopClients: 200, excluded: map[string]struct{}{}}
	item := database.QueryLogItem{
		Timestamp:   time.Now(),
		ClientIP:    "203.0.113.9",
		AccountName: "Public",
		Protocol:    "DoT",
		Domain:      "ads.example.com",
		RuleMatched: "Blocklist: AdAway",
		Action:      "BLOCK",
		QType:       1,
		Rcode:       0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.record(item, cfg, nil)
	}
}
