package service

import (
	"fmt"
	"testing"

	"digitdns/internal/database"
)

// The rate limiter answers a flood with silence, so its counters are the only
// evidence it fired. They reach the panel through a setter rather than a
// constructor argument — the DNS handler is built after this service, because the
// handler takes it as its telemetry sink — so the wiring itself is worth pinning:
// a service with no source attached must report zeros, not panic on a nil call.
func TestGuardStatsAreZeroWithoutASource(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()

	st := s.GetLiveStats()
	if st.RateLimited != 0 || st.RateLimitQPS != 0 {
		t.Errorf("unwired service reported rate_limited=%d rate_limit_qps=%d, want 0 and 0",
			st.RateLimited, st.RateLimitQPS)
	}
}

func TestGuardStatsAreForwarded(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()

	calls := 0
	s.SetGuardStatsSource(func() (uint64, int) {
		calls++
		return 4217, 200
	})

	st := s.GetLiveStats()
	if st.RateLimited != 4217 {
		t.Errorf("rate_limited = %d, want 4217", st.RateLimited)
	}
	if st.RateLimitQPS != 200 {
		t.Errorf("rate_limit_qps = %d, want 200", st.RateLimitQPS)
	}
	if calls != 1 {
		t.Errorf("source called %d times per GetLiveStats, want 1", calls)
	}

	// A limiter that is off reports 0 qps, and the panel renders that as "off"
	// rather than as "limiting at zero queries per second".
	s.SetGuardStatsSource(func() (uint64, int) { return 0, 0 })
	if st := s.GetLiveStats(); st.RateLimitQPS != 0 || st.RateLimited != 0 {
		t.Errorf("disabled limiter reported rate_limited=%d rate_limit_qps=%d, want 0 and 0",
			st.RateLimited, st.RateLimitQPS)
	}
}

// UptimeSec is declared in the response and rendered by the dashboard and the
// TUI, and for several releases it was never assigned — the tile showed 0 for a
// daemon that had been up for a week. A fresh service is younger than a second,
// so the only claim that holds without sleeping is that the field is not
// negative and not garbage; the formatting of it is pinned in internal/tui.
func TestUptimeIsAssigned(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()
	if st := s.GetLiveStats(); st.UptimeSec < 0 {
		t.Errorf("uptime_sec = %d, want >= 0", st.UptimeSec)
	}
}

// The SNI proxy's two drop counters travel the same setter path as the limiter's,
// for the same reason: the wiring is the part that breaks. RefusedCount existed
// for two releases with no caller anywhere in the repository, so the number was
// collected on every dropped connection and read by nothing — which is the same
// as not having it.
func TestProxyGuardStatsAreZeroWithoutASource(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()

	st := s.GetLiveStats()
	if st.RelaysRefused != 0 || st.RelaysUnreadable != 0 {
		t.Errorf("unwired service reported relays_refused=%d relays_unreadable=%d, want 0 and 0",
			st.RelaysRefused, st.RelaysUnreadable)
	}
}

func TestProxyGuardStatsAreForwarded(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()

	calls := 0
	s.SetProxyGuardStatsSource(func() (uint64, uint64) {
		calls++
		return 91, 7
	})

	st := s.GetLiveStats()
	if st.RelaysRefused != 91 {
		t.Errorf("relays_refused = %d, want 91", st.RelaysRefused)
	}
	if st.RelaysUnreadable != 7 {
		t.Errorf("relays_unreadable = %d, want 7", st.RelaysUnreadable)
	}
	// Once per snapshot: the dashboard polls every two seconds, and a source that
	// was read twice per poll would double any rate an operator computes from it.
	if calls != 1 {
		t.Errorf("source called %d times per GetLiveStats, want 1", calls)
	}
}

func TestProxyGuardBreakdownIsForwarded(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()
	s.SetProxyGuardDetailSource(func() ProxyGuardStats {
		return ProxyGuardStats{
			Refused: 91, Unreadable: 7,
			Malformed: 80, Blocked: 1, Self: 2, Quota: 3, TableFull: 4, ResolveFailed: 1,
		}
	})

	st := s.GetLiveStats()
	if st.RelaysRefused != 91 || st.RelaysUnreadable != 7 {
		t.Fatalf("drops = (%d, %d), want (91, 7)", st.RelaysRefused, st.RelaysUnreadable)
	}
	if st.RelaysRefusedMalformed != 80 || st.RelaysRefusedBlocked != 1 ||
		st.RelaysRefusedSelf != 2 || st.RelaysRefusedQuota != 3 ||
		st.RelaysRefusedTableFull != 4 || st.RelaysRefusedResolve != 1 {
		t.Fatalf("breakdown not forwarded: %+v", st)
	}
}

// The proxy counters must not be confused with the relay counters. GetStats and
// GuardStats are separate sources on the proxy and separate fields here, because
// a refused connection is not a relay — reporting it as one would inflate the
// relay total on the busiest card in the panel.
func TestProxyRelayAndGuardStatsAreIndependent(t *testing.T) {
	s := NewStatsService(nil, func() (int64, uint64, uint64, uint64) {
		return 3, 1200, 4096, 8192
	}, nil)
	defer s.Close()
	s.SetProxyGuardStatsSource(func() (uint64, uint64) { return 91, 7 })

	st := s.GetLiveStats()
	if st.ActiveRelays != 3 || st.TotalRelays != 1200 {
		t.Errorf("relays = (%d active, %d total), want (3, 1200)", st.ActiveRelays, st.TotalRelays)
	}
	if st.BytesSent != 4096 || st.BytesRecv != 8192 {
		t.Errorf("bytes = (%d sent, %d recv), want (4096, 8192)", st.BytesSent, st.BytesRecv)
	}
	if st.RelaysRefused != 91 || st.RelaysUnreadable != 7 {
		t.Errorf("drops = (%d refused, %d unreadable), want (91, 7)",
			st.RelaysRefused, st.RelaysUnreadable)
	}
}

// The ring is read back by every new SSE subscriber, so its order and its wrap
// are part of the live-log contract. The old slice-shift implementation kept the
// newest hundred but reallocated the window periodically; this pins the
// replacement's chronological output across more than one full wrap.
func TestRecentLogsRingKeepsNewestEntriesInOrder(t *testing.T) {
	s := NewStatsService(nil, nil, nil)
	defer s.Close()

	const extra = 7
	total := recentLogsCap + extra
	for i := 0; i < total; i++ {
		s.PushQueryLog(database.QueryLogItem{Domain: fmt.Sprintf("q-%d.example", i)})
	}

	logs := s.GetRecentLogs()
	if len(logs) != recentLogsCap {
		t.Fatalf("len(GetRecentLogs()) = %d, want %d", len(logs), recentLogsCap)
	}
	if got, want := logs[0].Domain, fmt.Sprintf("q-%d.example", extra); got != want {
		t.Errorf("oldest retained = %q, want %q", got, want)
	}
	if got, want := logs[len(logs)-1].Domain, fmt.Sprintf("q-%d.example", total-1); got != want {
		t.Errorf("newest retained = %q, want %q", got, want)
	}
}
