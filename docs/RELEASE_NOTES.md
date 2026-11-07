# DigitDNS v5.2.11 — faster panel, Prometheus metrics and a rule tester

**The panel gets faster.** Asset URLs carry a content fingerprint
(`/js/app.js?v=…`), and a matching request is now served with
`Cache-Control: public, max-age=31536000, immutable`. Repeat visits stop
revalidating every script, stylesheet and font. The dashboard document and any
stale fingerprint still revalidate, so an upgrade is never pinned to an old
bundle. The dashboard also polls `/api/stats` once a second only while the
Dashboard tab is open, and every five seconds elsewhere.

**Two operator tools.**

- **Prometheus metrics** — `GET /api/metrics`, guarded by the master REST key (or
  a dashboard session), serves the standard text format: build info, query
  totals and QPS, cache entries/hits/misses, refresh counters, relay traffic,
  CPU/RAM, goroutines and latency quantiles. Labels stay bounded: no domains,
  client addresses or account names.
- **Rule tester** — `GET /api/filter/test?domain=…` (and the v3 equivalent)
  answers what the live matcher would do with a name, including the matching
  list and whether an exact entry or a parent wildcard applied. It is read-only
  and has a card in the web Ad Blocking tab and the Android Ad Blocking screen.

**Security.** The CSP no longer allows `script-src 'unsafe-inline'`. The two
dashboard inline boot scripts, and the login script whose admin namespace is
spliced in per install, are authorised by SHA-256 hashes. An injected `<script>`
is refused even if it reaches the markup.

**Android.** Ordinary API calls have a 20-second read timeout again; only the SSE
client disables it, so a stalled stats call can no longer hang a coroutine. The
upstream resolver rows no longer show the amber `transport … · policy … · last: …`
line — the status dot and latency colour remain.

No database migration. No resolver or relay behaviour changes.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.11
docker pull ghcr.io/digitrob/digitdns-agent:5.2.11
docker pull robmeister/digitdns:5.2.11
docker pull robmeister/digitdns-agent:5.2.11
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.2.11/scripts/install.sh | sudo bash
```

The container in-app updater still reports `supported:false` and tells the
operator to pull and recreate; systemd installs keep the verified binary update.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.11-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.11` tag.
