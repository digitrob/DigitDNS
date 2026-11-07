# DigitDNS v5.2.3 — faster resolution and clearer diagnostics

**A performance and operations release.** The resolver defaults to Hedged
upstream racing, reuses DoT/DoQ connections, and loads history and settings with
far fewer requests. The SNI proxy and upstream cards now explain why
connections or answers fail, and a circuit breaker stops repeatedly paying for a
dead upstream.

There are no database migrations and no breaking API changes.

## Resolver and upstream performance

- Default racing strategy is **Hedged**; Fastest remains selectable. A cache
  miss no longer fans out to every upstream by default.
- Persistent **DoT connection pool** and **DoQ connection reuse**. A pooled
  connection that the server closed while idle is retried once on a fresh one
  instead of surfacing as a query failure.
- TLS 1.3 session cache shared by DoT/DoH/DoQ; DoH idle and per-host connection
  limits tuned for reuse.
- Immutable query-path snapshot, pre-filtered transport candidates and a cached
  latency order, removing per-miss slice copies, settings locks and map/sort
  work.
- Relay targets carry pre-parsed IPs; the rate limiter uses allocation-free
  `netip.ParseAddr`.
- Tests pin five DoT queries to one TLS connection and five DoQ queries to one
  QUIC connection.

## Faster panel and Android

- New `GET /api/v3/stats/history` returns the summary, series and all six ranked
  tables in one response. Web and Android history moved from **8 requests to 1**.
- `GET /api/v3/stats` now carries the upstream rows; Android no longer makes a
  second three-second poll for them.
- Web boot defers Agents, DNSSEC and Proxy Leak loads until their tab opens.
- Chart.js loads on demand from the admin namespace, so the first screen paints
  before the chart library.

## Diagnostics

- SNI proxy refusal breakdown: **malformed**, **blocked**, **self-dial**,
  **quota**, **table-full** and **resolution failure**. The reasons are exposed
  in `/api/stats` and the Refused tooltip; malformed scanner probes no longer
  look like relay failures.
- Upstream errors are split into **transport failures** and **policy rcodes**
  (`REFUSED`, `SERVFAIL`, etc.), with `last_error`, `last_error_at`,
  `consecutive_failures` and `circuit_open` in the live stats.
- **Circuit breaker**: opens after five consecutive transport failures, stays
  open for 30 seconds, is reset by a success or a policy rcode, and fails open
  when every candidate is open.
- The web and Android show the state compactly: a red/amber dot and a detailed
  tooltip, with no extra card line or chip.

## Upgrade

The in-panel self-updater and the one-line installer use the same release.

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

Docker Hub mirror:

```bash
docker pull robmeister/digitdns:latest
docker pull robmeister/digitdns-agent:latest
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.3
docker pull robmeister/digitdns:5.2.3
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.3-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.3` tag.
