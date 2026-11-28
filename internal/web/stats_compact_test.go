package web

import (
	"net/http"
	"slices"
	"testing"

	"digitdns/internal/core/upstream"
)

func statsSortedKeys(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// TestStatsCompactContracts pins the three wire shapes. compact=1 is the
// legacy delete-list form; compact=2 is the dashboard's whitelist heartbeat.
// Changing either without a deliberate client update must fail here instead of
// quietly changing what a 1 Hz poll costs.
func TestStatsCompactContracts(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	get := func(path string) (map[string]any, int) {
		w := authedGet(t, h, path, tok)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d — %s", path, w.Code, w.Body.String())
		}
		return decodeBody(t, w), w.Body.Len()
	}

	full, fullLen := get("/api/stats")
	legacy, legacyLen := get("/api/stats?compact=1")
	compact, compactLen := get("/api/stats?compact=2")

	for _, key := range []string{
		"ram_usage_mb", "cpu_usage_percent", "system_cpu_percent",
		"num_goroutines", "uptime_sec", "panics_total",
		"agent_catalogue_requests", "agent_config_revision",
	} {
		if _, ok := full[key]; !ok {
			t.Errorf("full stats no longer carry %q", key)
		}
		if _, ok := legacy[key]; ok {
			t.Errorf("legacy compact still carries slow field %q", key)
		}
		if _, ok := compact[key]; ok {
			t.Errorf("compact=2 still carries slow field %q", key)
		}
	}

	// The legacy aliases were never painted by the panel, but older clients may
	// still read them; compact=1 must remain byte-compatible for those clients.
	for _, key := range []string{"cache_hits", "cache_misses", "bytes_sent", "bytes_recv"} {
		if _, ok := legacy[key]; !ok {
			t.Errorf("legacy compact lost %q", key)
		}
		if _, ok := compact[key]; ok {
			t.Errorf("compact=2 carries unused alias %q", key)
		}
	}

	wantCompact := []string{
		"active_proxy_conns", "cache_entries", "cache_hit_ratio",
		"latency", "latency_uncached", "latency_range", "qps", "rate_limit_qps",
		"rate_limited", "refresh_dropped", "refresh_failed",
		"refresh_started", "relays_refused", "relays_refused_blocked",
		"relays_refused_flood", "relays_refused_malformed",
		"relays_refused_quota", "relays_refused_resolve",
		"relays_refused_self", "relays_refused_table_full",
		"relays_unreadable", "relays_quic_rejected", "relays_quic_ignored",
		"relays_quic_limited", "speed_in_kbps", "speed_out_kbps",
		"stale_served", "total_bytes_transferred", "total_proxy_conns",
		"total_queries", "upstream_metrics", "upstreams",
	}
	slices.Sort(wantCompact)
	if got := statsSortedKeys(compact); !slices.Equal(got, wantCompact) {
		t.Fatalf("compact=2 keys = %v, want %v", got, wantCompact)
	}
	if compactLen >= legacyLen || legacyLen >= fullLen {
		t.Fatalf("payload sizes not ordered: full=%d legacy=%d compact2=%d", fullLen, legacyLen, compactLen)
	}
}

// TestCompactUpstreamStatsKeepsRenderedFields pins the smaller upstream rows.
// A field the card reads must survive; a redundant percentile/average field
// must not. Zero values are kept so a counter returning to zero cannot leave a
// stale number behind when the browser merges a heartbeat over a full poll.
func TestCompactUpstreamStatsKeepsRenderedFields(t *testing.T) {
	stats := []upstream.UpstreamStat{{
		Address:             "1.1.1.1:53",
		LatencyNs:           12_000_000,
		LatencyMs:           12,
		LatencyP50Ms:        11,
		LatencyP95Ms:        18,
		LatencyAvgMs:        13,
		Measured:            true,
		Samples:             9,
		LastMeasuredAt:      "2026-11-20T07:00:00Z",
		LatencyIsProbe:      false,
		Hits:                10,
		Errors:              1,
		TransportErrors:     2,
		PolicyErrors:        3,
		RecentSamples:       4,
		RecentErrorRate:     0.25,
		ConsecutiveFailures: 1,
		CircuitState:        "closed",
		LastError:           "",
		ProbeHits:           1,
		ProbeErrors:         0,
		ProbePolicy:         0,
		ProbeSamples:        2,
		LastProbeAt:         "2026-11-20T06:59:00Z",
		LastProbeMs:         12,
		ProbeMedianMs:       11,
	}}

	got := compactUpstreamStats(stats)
	if len(got) != 1 {
		t.Fatalf("compactUpstreamStats returned %d rows, want 1", len(got))
	}
	wantKeys := []string{
		"address", "circuit_open", "circuit_state", "consecutive_failures",
		"errors", "hits", "last_error", "last_measured_at", "last_probe_at",
		"latency", "latency_is_probe", "latency_p95_ms", "measured",
		"policy_errors", "probe_errors", "probe_hits", "probe_policy",
		"probe_samples", "recent_error_rate", "recent_samples", "samples",
		"transport_errors",
	}
	slices.Sort(wantKeys)
	if keys := statsSortedKeys(got[0]); !slices.Equal(keys, wantKeys) {
		t.Fatalf("compact upstream keys = %v, want %v", keys, wantKeys)
	}
	for _, key := range []string{"latency_ms", "latency_p50_ms", "latency_avg_ms", "last_probe_ms", "probe_median_ms"} {
		if _, ok := got[0][key]; ok {
			t.Errorf("compact upstream still carries %q", key)
		}
	}
	if got[0]["latency_p95_ms"] != 18.0 {
		t.Errorf("latency_p95_ms = %v, want 18", got[0]["latency_p95_ms"])
	}
	if open, ok := got[0]["circuit_open"]; !ok || open != false {
		t.Errorf("circuit_open = %v,%v, want false,true", open, ok)
	}
	if err := got[0]["last_error"]; err != "" {
		t.Errorf("last_error = %v, want an explicit empty string", err)
	}
}
