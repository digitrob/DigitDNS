# DigitDNS v5.4.3 — faster dashboard and clearer state

**A performance and UX patch: less JavaScript and network on the dashboard, and
the panel now names what is wrong when a listener or upstream is unhealthy.**

No resolver, API or database schema changes are required. Existing settings,
clients, blocklists and certificates are preserved.

## Dashboard performance

- **API responses are gzip-compressed** when the client accepts it. On a live
  install this cut `/api/filtering` from ~22 KB to ~5.3 KB, `/api/metrics` from
  ~7.7 KB to ~1.7 KB, and `/api/config` from ~2.2 KB to ~0.9 KB. Responses under
  512 bytes stay identity-encoded, so tiny bodies do not grow.
- **Minified dashboard assets** are served under the existing
  `/js/app.js` and `/css/style.css` URLs. `app.js` drops from 551 KB to 298 KB
  raw and from ~153 KB to ~80 KB gzipped; `style.css` drops from 134 KB to
  58 KB raw and ~39 KB to ~11.8 KB gzipped. The readable source stays embedded
  and a source-hash test catches stale minified output.
- **Chart.js is deferred** until the first stats poll instead of loading on
  `DOMContentLoaded`. The login page no longer loads it at all.
- **Less layout work per poll:** the stats tiles and latency figures use
  `textContent` rather than `innerText`, and the QPS chart repaints at most once
  per ~1.5 seconds instead of every second.

## Clearer state

- **Global listener notice:** if DoH3 or DoQ is enabled but not listening, every
  tab now shows the listener name, the server's `last_error`, and an
  **Open transports** action.
- **Global upstream notice:** degraded upstream resolvers are named at the top
  of the panel, with a **Review upstreams** action. This uses the same health
  predicate as the monitor: open circuit, failed latest probe, or a recent error
  rate of at least 10% over 20 queries.
- **“Why was this blocked?”** A blocked query-log row now shows the exact source
  under the domain, for example `Blocked by Custom Block · ads.example.com` or
  `Blocked by Blocklist: <list> · <pattern>`. It is visible on phones, where the
  separate RULE column is hidden.
- **Access-mode reporting fixed:** `/api/config/access` now reports the live
  access mode instead of an older `access.allow_all` copy. The dashboard and the
  resolver can no longer disagree about whether public mode is on.
- The full-width public-mode banner was removed; the Clients page owns that
  switch, and the persistent banner duplicated it across the panel.

## Android

Android only carries the version bump to **5.4.3 (540)**. No Android behaviour
changes are included in this patch.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.4.3
docker pull ghcr.io/digitrob/digitdns-agent:5.4.3
docker pull robmeister/digitdns:5.4.3
docker pull robmeister/digitdns-agent:5.4.3
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.4.3/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.4.3-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.4.3` tag.
