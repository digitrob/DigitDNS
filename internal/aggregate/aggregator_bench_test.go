package aggregate

import (
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkRecordTotal measures the query-count hot path. RecordQuery calls it
// once per DNS packet before any detail work, so contention here is paid by
// every query even when no dashboard is open.
func BenchmarkRecordTotal(b *testing.B) {
	benchmarkRecordTotal(b, false)
}

// BenchmarkRecordTotalParallel is the same path under the concurrency a busy
// resolver actually produces. The sharded implementation should scale with
// GOMAXPROCS instead of serialising every query on one bucket mutex.
func BenchmarkRecordTotalParallel(b *testing.B) {
	benchmarkRecordTotal(b, true)
}

func benchmarkRecordTotal(b *testing.B, parallel bool) {
	now := time.Now()
	a, err := Open(filepath.Join(b.TempDir(), "stats.db"), DefaultConfig())
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	b.Cleanup(func() { a.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				a.RecordTotal(now)
			}
		})
		return
	}
	for i := 0; i < b.N; i++ {
		a.RecordTotal(now)
	}
}
