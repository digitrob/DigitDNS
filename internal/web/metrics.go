package web

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"digitdns/internal/httpx"
	"digitdns/internal/service"
	"digitdns/internal/version"
)

// TransportMetrics is the QUIC transport's scrape surface. It is a separate
// type from dns.TransportMetrics so the web package does not have to import the
// resolver package just to read counters.
type TransportMetrics struct {
	DoH3Requests         uint64
	DoH3Connections      int64
	DoH3ConnectionsTotal uint64
	DoH3HandshakeRejects uint64

	DoQQueries          uint64
	DoQConnections      int64
	DoQConnectionsTotal uint64
	DoQAuthFailures     uint64
	DoQProtocolErrors   uint64
	DoQHandshakeRejects uint64
}

// Metrics is the Prometheus scrape surface. It is scratch-built rather than
// pulling in the client library: the exposition format is a few lines of text,
// and this project keeps its module graph small on purpose.
//
// Authorization is the master REST key (or a live dashboard session). Prometheus
// sends the key as `Authorization: Bearer <key>`; the same key already guards the
// v2/v3 APIs, so no second credential has to be stored or rotated.
//
// Cardinality is deliberately bounded: labels are fixed vocabularies (version,
// channel) and quantiles. No domain names, client addresses or account names —
// those belong to the dashboard, not to a time-series database.
func (ws *WebServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	if ws.stats == nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}

	st := ws.stats.GetLiveStats()
	info := version.Get()

	var b strings.Builder
	writeMetric := func(name, help, kind string, value any) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n%s %v\n", name, help, name, kind, name, value)
	}

	// A constant 1 carrying build labels: the standard way to join a dashboard to
	// the running version.
	b.WriteString("# HELP digitdns_build_info Build identity of the running daemon.\n")
	b.WriteString("# TYPE digitdns_build_info gauge\n")
	fmt.Fprintf(&b, "digitdns_build_info{version=%q,channel=%q,hash=%q,go=%q} 1\n",
		info.Version, info.Channel, info.Hash, runtime.Version())

	writeMetric("digitdns_uptime_seconds", "Seconds since the daemon started.", "gauge", st.UptimeSec)
	writeMetric("digitdns_queries_total", "Queries answered since start.", "counter", st.TotalQueries)
	writeMetric("digitdns_qps", "Queries per second over the current window.", "gauge", st.QPS)
	writeMetric("digitdns_rate_limited_total", "Queries refused by the per-source rate limiter.", "counter", st.RateLimited)
	writeMetric("digitdns_panics_total", "Panics recovered at a request boundary.", "counter", st.PanicsTotal)

	writeMetric("digitdns_cache_entries", "Entries currently held in the DNS cache.", "gauge", st.CacheItems)
	writeMetric("digitdns_cache_hits_total", "DNS cache hits since start.", "counter", st.CacheHits)
	writeMetric("digitdns_cache_misses_total", "DNS cache misses since start.", "counter", st.CacheMisses)
	writeMetric("digitdns_cache_hit_ratio", "Cache hit ratio in the range 0..1.", "gauge", st.CacheHitRate/100.0)

	writeMetric("digitdns_stale_served_total", "Expired entries served while a refresh ran.", "counter", st.StaleServed)
	writeMetric("digitdns_refresh_started_total", "Background cache refreshes started.", "counter", st.RefreshStarted)
	writeMetric("digitdns_refresh_failed_total", "Background cache refreshes that failed.", "counter", st.RefreshFailed)
	writeMetric("digitdns_refresh_dropped_total", "Refreshes skipped because the pool was saturated.", "counter", st.RefreshDropped)

	writeMetric("digitdns_relays_active", "SNI relay connections currently open.", "gauge", st.ActiveRelays)
	writeMetric("digitdns_relays_total", "SNI relay connections since start.", "counter", st.TotalRelays)
	writeMetric("digitdns_relay_bytes_sent_total", "Bytes sent through the SNI relay.", "counter", st.BytesSent)
	writeMetric("digitdns_relay_bytes_received_total", "Bytes received through the SNI relay.", "counter", st.BytesRecv)
	writeMetric("digitdns_relays_refused_total", "Relay connections refused, including flood attempts from a source already in the malformed-probe penalty box.", "counter", st.RelaysRefused)
	actionableRefused := st.RelaysRefused
	if st.RelaysRefusedFlood > actionableRefused {
		actionableRefused = 0
	} else {
		actionableRefused -= st.RelaysRefusedFlood
	}
	writeMetric("digitdns_relays_refused_actionable_total", "Relay connections refused for an actionable reason (flood attempts excluded).", "counter", actionableRefused)
	writeMetric("digitdns_relays_flood_total", "Relay connection attempts from a source already in the malformed-probe penalty box.", "counter", st.RelaysRefusedFlood)

	var tm TransportMetrics
	if ws.transportMetrics != nil {
		tm = ws.transportMetrics()
	}
	writeMetric("digitdns_doh3_requests_total", "HTTP/3 DoH requests served since start.", "counter", tm.DoH3Requests)
	writeMetric("digitdns_doh3_connections", "HTTP/3 DoH connections currently open.", "gauge", tm.DoH3Connections)
	writeMetric("digitdns_doh3_connections_total", "HTTP/3 DoH connections accepted since start.", "counter", tm.DoH3ConnectionsTotal)
	writeMetric("digitdns_doq_queries_total", "DNS-over-QUIC queries served since start.", "counter", tm.DoQQueries)
	writeMetric("digitdns_doq_connections", "DNS-over-QUIC connections currently open.", "gauge", tm.DoQConnections)
	writeMetric("digitdns_doq_connections_total", "DNS-over-QUIC connections accepted since start.", "counter", tm.DoQConnectionsTotal)
	writeMetric("digitdns_doq_auth_failures_total", "DNS-over-QUIC connections rejected by SNI-token authentication.", "counter", tm.DoQAuthFailures)
	writeMetric("digitdns_doq_protocol_errors_total", "DNS-over-QUIC streams closed for a protocol violation.", "counter", tm.DoQProtocolErrors)
	b.WriteString("# HELP digitdns_quic_handshake_rejected_total New QUIC connections refused before the handshake, by transport.\n")
	b.WriteString("# TYPE digitdns_quic_handshake_rejected_total counter\n")
	fmt.Fprintf(&b, "digitdns_quic_handshake_rejected_total{transport=%q} %d\n", "doh3", tm.DoH3HandshakeRejects)
	fmt.Fprintf(&b, "digitdns_quic_handshake_rejected_total{transport=%q} %d\n", "doq", tm.DoQHandshakeRejects)
	writeMetric("digitdns_relays_unreadable_total", "Relay connections whose opening bytes named no destination.", "counter", st.RelaysUnreadable)

	writeMetric("digitdns_cpu_percent", "Process CPU usage percentage.", "gauge", st.CPUUsagePercent)
	writeMetric("digitdns_system_cpu_percent", "Machine-wide CPU usage percentage.", "gauge", st.SystemCPUPercent)
	writeMetric("digitdns_memory_alloc_bytes", "Bytes of allocated heap objects.", "gauge", st.AllocMemoryMB*1024*1024)
	writeMetric("digitdns_memory_sys_bytes", "Bytes obtained from the OS.", "gauge", st.SysMemoryMB*1024*1024)
	writeMetric("digitdns_system_memory_used_bytes", "Machine-wide used memory in bytes.", "gauge", st.SystemMemUsedMB*1024*1024)
	writeMetric("digitdns_system_memory_total_bytes", "Machine-wide total memory in bytes.", "gauge", st.SystemMemTotalMB*1024*1024)
	writeMetric("digitdns_goroutines", "Goroutines currently running.", "gauge", st.NumGoroutines)

	// Relay-agent control plane. The catalogue counters are the ones to watch
	// after a hub restart: a spike in builds with no config change means agents
	// were invalidated unnecessarily.
	writeMetric("digitdns_agent_catalogue_requests_total", "Agent catalogue requests received.", "counter", ws.agentCatRequests.Load())
	writeMetric("digitdns_agent_catalogue_not_modified_total", "Agent catalogue requests answered 304.", "counter", ws.agentCatNotMod.Load())
	writeMetric("digitdns_agent_catalogue_builds_total", "Agent catalogues rebuilt (full fetch).", "counter", ws.agentCatBuilds.Load())
	writeMetric("digitdns_agent_catalogue_long_polls", "Long-poll catalogue requests currently parked.", "gauge", ws.agentCatParked.Load())
	writeMetric("digitdns_agent_revision", "Current agent control-plane revision.", "gauge", ws.agentRev.Load())
	online, total := ws.agentFleetCounts()
	writeMetric("digitdns_agents_online", "Agents inside the heartbeat window.", "gauge", online)
	writeMetric("digitdns_agents_total", "Configured relay agents.", "gauge", total)

	// Latency as summary quantiles. The histogram buckets behind these numbers are
	// already maintained for the dashboard, so the scrape reuses them rather than
	// keeping a second set for Prometheus.
	b.WriteString("# HELP digitdns_latency_ms Query service time in milliseconds.\n")
	b.WriteString("# TYPE digitdns_latency_ms summary\n")
	for _, q := range []struct {
		label string
		value float64
	}{{"0.5", st.Latency.P50Ms}, {"0.95", st.Latency.P95Ms}, {"0.99", st.Latency.P99Ms}} {
		fmt.Fprintf(&b, "digitdns_latency_ms{quantile=%q,cached=%q} %v\n", q.label, "all", q.value)
	}
	for _, q := range []struct {
		label string
		value float64
	}{{"0.5", st.LatencyUncached.P50Ms}, {"0.95", st.LatencyUncached.P95Ms}, {"0.99", st.LatencyUncached.P99Ms}} {
		fmt.Fprintf(&b, "digitdns_latency_ms{quantile=%q,cached=%q} %v\n", q.label, "never", q.value)
	}
	fmt.Fprintf(&b, "digitdns_latency_ms_count %d\n", st.Latency.Count)
	fmt.Fprintf(&b, "digitdns_latency_ms_sum %v\n", st.Latency.AvgMs*float64(st.Latency.Count))

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}

// agentFleetCounts reports how many configured agents are inside the same
// heartbeat window the relay router uses. The result is cached for five seconds:
// a Prometheus scrape should not walk the whole agent bucket every time.
func (ws *WebServer) agentFleetCounts() (int, int) {
	if ws.db == nil {
		return 0, 0
	}
	now := time.Now()
	ws.agentFleetMu.Lock()
	if !ws.agentFleetAt.IsZero() && now.Sub(ws.agentFleetAt) < 5*time.Second {
		online, total := ws.agentFleetOnline, ws.agentFleetTotal
		ws.agentFleetMu.Unlock()
		return online, total
	}
	ws.agentFleetMu.Unlock()

	agents, err := ws.db.ListAgents()
	if err != nil {
		return 0, 0
	}
	online := 0
	for _, a := range agents {
		if a.Enabled && !a.LastSeen.IsZero() && now.Sub(a.LastSeen) <= service.AgentOnlineFor {
			online++
		}
	}
	ws.agentFleetMu.Lock()
	ws.agentFleetAt = now
	ws.agentFleetOnline = online
	ws.agentFleetTotal = len(agents)
	ws.agentFleetMu.Unlock()
	return online, len(agents)
}
