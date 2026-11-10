# Monitoring DigitDNS with Prometheus and Grafana

DigitDNS exposes a Prometheus scrape endpoint at:

```
GET https://<host>:<port>/<admin-path>/api/metrics
```

It answers the Prometheus text exposition format
(`text/plain; version=0.0.4`). Authentication is the master REST key (or a live
dashboard session), sent the same way as the other APIs:

```http
GET /<admin-path>/api/metrics HTTP/1.1
Host: dns.example.com:27586
Authorization: Bearer digitdns_live_your_key
```

Bind the API to the scraper. If `api_bind` is `127.0.0.1` (the default), run
Prometheus on the same host or through an SSH tunnel; otherwise add the scraper
to **Settings → API Network Binding**.

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: digitdns
    scheme: https
    metrics_path: /<admin-path>/api/metrics
    authorization:
      credentials: digitdns_live_your_key
    static_configs:
      - targets: ["dns.example.com:27586"]
    scrape_interval: 15s
    scrape_timeout: 10s
```

## Metrics

| Metric | Type | Meaning |
| :--- | :--- | :--- |
| `digitdns_build_info` | gauge | Constant `1` with `version`, `channel`, `hash` and `go` labels. |
| `digitdns_uptime_seconds` | gauge | Seconds since the daemon started. |
| `digitdns_queries_total` | counter | Queries answered since start. |
| `digitdns_qps` | gauge | Queries per second over the current window. |
| `digitdns_rate_limited_total` | counter | Queries refused by the per-source limiter. |
| `digitdns_panics_total` | counter | Panics recovered at a request boundary. |
| `digitdns_upstreams_degraded` | gauge | Configured upstream resolvers the health monitor currently considers degraded: open circuit, failed latest probe, or a recent live error rate of at least 10% over 20 queries. |
| `digitdns_cache_entries` | gauge | Entries in the DNS cache. |
| `digitdns_cache_hits_total` | counter | DNS cache hits. |
| `digitdns_cache_misses_total` | counter | DNS cache misses. |
| `digitdns_cache_hit_ratio` | gauge | Hit ratio, 0..1. |
| `digitdns_stale_served_total` | counter | Expired entries served during a refresh. |
| `digitdns_refresh_started_total` | counter | Background cache refreshes started. |
| `digitdns_refresh_failed_total` | counter | Background refreshes that failed. |
| `digitdns_refresh_dropped_total` | counter | Refreshes skipped because the pool was saturated. |
| `digitdns_relays_active` | gauge | SNI relay connections open. |
| `digitdns_relays_total` | counter | SNI relay connections since start. |
| `digitdns_relay_bytes_sent_total` | counter | Bytes sent through the relay. |
| `digitdns_relay_bytes_received_total` | counter | Bytes received through the relay. |
| `digitdns_relays_refused_total` | counter | Relay connections refused, including flood attempts from a source already in the malformed-probe penalty box. |
| `digitdns_relays_refused_actionable_total` | counter | Relay connections refused for an actionable reason (flood attempts excluded). |
| `digitdns_relays_flood_total` | counter | Relay connection attempts from a source already in the malformed-probe penalty box. |
| `digitdns_relays_unreadable_total` | counter | Connections whose opening bytes named no destination. |
| `digitdns_doh3_requests_total` | counter | HTTP/3 DoH requests served since start. |
| `digitdns_doh3_connections` | gauge | HTTP/3 DoH connections currently open. |
| `digitdns_doh3_connections_total` | counter | HTTP/3 DoH connections accepted since start. |
| `digitdns_doq_queries_total` | counter | DNS-over-QUIC queries served since start. |
| `digitdns_doq_connections` | gauge | DNS-over-QUIC connections currently open. |
| `digitdns_doq_connections_total` | counter | DNS-over-QUIC connections accepted since start. |
| `digitdns_doq_auth_failures_total` | counter | DoQ connections rejected by SNI-token authentication. |
| `digitdns_doq_protocol_errors_total` | counter | DoQ streams closed for a protocol violation (bad length, query framing or nonzero transaction ID). |
| `digitdns_quic_handshake_rejected_total` | counter | QUIC handshake attempts refused before TLS, by `transport` (`doh3`/`doq`). Covers the per-source rate limit and the 1,024-connection cap. |
| `digitdns_cpu_percent` | gauge | Process CPU usage. |
| `digitdns_system_cpu_percent` | gauge | Machine-wide CPU usage. |
| `digitdns_memory_alloc_bytes` | gauge | Allocated heap bytes. |
| `digitdns_memory_sys_bytes` | gauge | Bytes obtained from the OS. |
| `digitdns_system_memory_used_bytes` | gauge | Machine-wide used memory. |
| `digitdns_system_memory_total_bytes` | gauge | Machine-wide total memory. |
| `digitdns_goroutines` | gauge | Goroutines running. |
| `digitdns_latency_ms` | summary | Query service time by `quantile` (0.5/0.95/0.99) and `cached` (`all`/`never`). |
| `digitdns_agent_catalogue_requests_total` | counter | Agent catalogue requests received. |
| `digitdns_agent_catalogue_not_modified_total` | counter | Catalogue requests answered `304`. |
| `digitdns_agent_catalogue_builds_total` | counter | Catalogues rebuilt (full fetch). |
| `digitdns_agent_catalogue_long_polls` | gauge | Long-poll catalogue requests parked. |
| `digitdns_agent_revision` | gauge | Current control-plane revision. |
| `digitdns_agents_online` | gauge | Agents inside the heartbeat window (90 s). |
| `digitdns_agents_total` | gauge | Configured relay agents. |

Labels are deliberately bounded: version/channel, cache mode and latency
quantiles. No domains, client addresses or account names ever reach the metrics.

## PromQL starting points

```promql
# Queries per second, from the counter rather than the gauge
rate(digitdns_queries_total[5m])

# Cache efficiency
digitdns_cache_hit_ratio

# p95 latency, all queries vs upstream-bound queries
digitdns_latency_ms{quantile="0.95",cached="all"}
digitdns_latency_ms{quantile="0.95",cached="never"}

# Agent catalogue 304 ratio: should stay near 1 in steady state
sum(rate(digitdns_agent_catalogue_not_modified_total[5m]))
  / sum(rate(digitdns_agent_catalogue_requests_total[5m]))

# Rebuilds in the last 10 minutes: should be zero across a hub restart unless
# the config changed (or the binary was upgraded)
increase(digitdns_agent_catalogue_builds_total[10m])

# Agents currently online
digitdns_agents_online

# Upstreams currently considered degraded: should be zero in steady state
digitdns_upstreams_degraded

# QUIC connections in use, by transport (DoH3 multiplexes requests, so this is
# usually small even under load)
digitdns_doh3_connections
digitdns_doq_connections

# Handshake refusals: should be zero in steady state. A sustained non-zero rate
# means a source is exhausting its per-IP budget or the connection cap is hit.
sum by (transport) (rate(digitdns_quic_handshake_rejected_total[5m]))

# DoQ streams closed for malformed framing — scanners, not subscribers.
rate(digitdns_doq_protocol_errors_total[5m])
```

A restart with the same build and unchanged config should produce **304s and no
builds**. A build spike after a restart means the control-plane revision was not
persisted (pre-5.2.13) or the config changed.

## Grafana

Import [`grafana/digitdns.json`](grafana/digitdns.json) in Grafana
(**Dashboards → New → Import**), then pick the Prometheus datasource when asked.
The dashboard covers queries, cache, latency, relay traffic, process/machine
resources and the agent control plane.
