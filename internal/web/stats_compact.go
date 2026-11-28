package web

import (
	"digitdns/internal/core/upstream"
)

// compactStatsV2Keys is the explicit contract for the dashboard's 1 Hz poll.
//
// compact=1 is still supported for older clients: it starts from the full
// telemetry document and deletes the slow fields. That is a safe but fragile
// definition — every new full field is automatically carried on every poll.
// compact=2 is the inverse: only the fields the dashboard actually repaints go
// into the heartbeat. A future field added to the full document cannot bloat
// this response by accident, and the full snapshot every fifth poll still has
// every diagnostic count an operator can read.
var compactStatsV2Keys = map[string]struct{}{
	"qps":                       {},
	"total_queries":             {},
	"cache_entries":             {},
	"cache_hit_ratio":           {},
	"active_proxy_conns":        {},
	"total_proxy_conns":         {},
	"speed_in_kbps":             {},
	"speed_out_kbps":            {},
	"total_bytes_transferred":   {},
	"rate_limited":              {},
	"rate_limit_qps":            {},
	"stale_served":              {},
	"refresh_started":           {},
	"refresh_failed":            {},
	"refresh_dropped":           {},
	"relays_refused":            {},
	"relays_unreadable":         {},
	"relays_refused_malformed":  {},
	"relays_refused_blocked":    {},
	"relays_refused_self":       {},
	"relays_refused_quota":      {},
	"relays_refused_table_full": {},
	"relays_refused_resolve":    {},
	"relays_refused_flood":      {},
	"relays_quic_rejected":      {},
	"relays_quic_ignored":       {},
	"relays_quic_limited":       {},
	"latency":                   {},
	"latency_uncached":          {},
	"latency_range":             {},
	"upstreams":                 {},
	"upstream_metrics":          {},
}

// compactStatsV2 keeps only the heartbeat fields. The caller still builds the
// full snapshot first because that is the only place the values are collected;
// trimming happens before encoding, so the saved bytes are on the wire as well
// as in the browser's JSON parse.
func compactStatsV2(resp map[string]any) map[string]any {
	compact := make(map[string]any, len(compactStatsV2Keys))
	for key, value := range resp {
		if _, keep := compactStatsV2Keys[key]; keep {
			compact[key] = value
		}
	}
	return compact
}

// compactUpstreamStats drops the fields the dashboard never reads from a poll.
// Every field the resolver card can show is either kept or reset through a value
// it does send, so a counter returning to zero cannot leave a stale number on
// the card when the browser merges this over the last full snapshot.
func compactUpstreamStats(stats []upstream.UpstreamStat) []map[string]any {
	out := make([]map[string]any, 0, len(stats))
	for _, u := range stats {
		out = append(out, map[string]any{
			"address":              u.Address,
			"latency":              u.LatencyNs,
			"measured":             u.Measured,
			"samples":              u.Samples,
			"last_measured_at":     u.LastMeasuredAt,
			"latency_is_probe":     u.LatencyIsProbe,
			"hits":                 u.Hits,
			"errors":               u.Errors,
			"transport_errors":     u.TransportErrors,
			"policy_errors":        u.PolicyErrors,
			"recent_samples":       u.RecentSamples,
			"recent_error_rate":    u.RecentErrorRate,
			"consecutive_failures": u.ConsecutiveFailures,
			"circuit_open":         u.CircuitOpen,
			"circuit_state":        u.CircuitState,
			"last_error":           u.LastError,
			"probe_hits":           u.ProbeHits,
			"probe_errors":         u.ProbeErrors,
			"probe_policy":         u.ProbePolicy,
			"probe_samples":        u.ProbeSamples,
			"last_probe_at":        u.LastProbeAt,
			"latency_p95_ms":       u.LatencyP95Ms,
		})
	}
	return out
}
