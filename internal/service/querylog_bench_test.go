package service

import (
	"testing"

	"digitdns/internal/database"
)

// BenchmarkQueryLogPushDisabled measures the DNS hot-path cost when query
// logging is off — the default. Before the settings cache it paid a bbolt read
// plus a decrypt on every query; now it is a single atomic load and a nil/flag
// compare.
func BenchmarkQueryLogPushDisabled(b *testing.B) {
	s := NewQueryLogService(nil, "")
	item := database.QueryLogItem{Domain: "example.com", ClientIP: "203.0.113.1"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(item)
	}
}
