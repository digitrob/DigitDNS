# DigitDNS v4.1.0 — Traffic History

DigitDNS now keeps an optional **90-day aggregate DNS history** and presents it
as a first-class dashboard on web and Android. No raw query log is written to
disk: the store holds counters and bounded top-N tables only.

## Highlights

- **Hourly and daily aggregates.** Seven days of hourly buckets roll into daily
  history, which defaults to 90 days. Retention and top-N limits are configurable.
- **Six ranked tables.** Top domains, blocked domains, clients, upstreams,
  protocols and relay countries, plus queries, blocked/proxied/cached/failed
  totals, qtype/rcode, latency and cache health.
- **HLL distinct clients.** HyperLogLog registers give an unbounded distinct
  client count instead of clipping at the top-client limit. Ranges containing
  pre-HLL buckets say `top-N approx.`
- **Privacy controls.** Client anonymization uses HMAC-SHA256 with a per-store
  salt. Up to 500 clients can opt out by IP or account name; their queries still
  count, but no client identifier is stored.
- **Web history dashboard.** 24h / 7d / 30d / 90d range selector, KPI cards,
  stacked Allowed / Blocked / Proxied chart, ranked tables, CSV and JSON export.
  History cards participate in the existing layout editor — drag, resize, hide
  and persist alongside the live cards.
- **Android 4.1.0.** History cards on the dashboard, range switching, chart,
  ranked tables, relay flags, client names, and **Settings → Traffic History**
  for retention, top-N, anonymization and opt-out.
- **Useful list geometry.** Web and Android pin the card header and `Other N`
  footer; only the entry list scrolls. Web cards reveal more rows when resized
  taller. Ranked rows use the same inset tile treatment as the upstream-resolvers
  card. Android shows five rows for domains/blocked/upstreams and three for
  clients/protocols/relay countries; Protocols and Relay countries share a
  half-width row.
- **Names and flags.** Top-client rows resolve a stored IP to the client’s
  current name (falling back to the IP when unknown). Relay-country rows show
  the flag and localized country name.
- **API v3.** `GET /api/v3/stats/{summary,series,top,export}` and
  `GET/POST /api/v3/stats/settings`, all documented in OpenAPI and available to
  either a dashboard session or the master API key.

## Privacy and storage

`stats.db` is created beside `data.db` with mode `0600`. It contains aggregates
only — no query payloads, no replayable log. Client opt-out accepts up to 500 IPs
or account names; anonymization applies to new aggregates and never rewrites
existing rows. The salt lives in `stats.db`, so it protects casual disclosure,
not an attacker who already has the database.

## Upgrade

Pull the new images and recreate the containers. Keep the existing `/app/data`
and `/app/certs` volumes; no migration is required. `stats.db` is created on
first start and starts collecting from that moment.

```bash
docker pull ghcr.io/digitrob/digitdns:4.1.0
docker pull ghcr.io/digitrob/digitdns-agent:4.1.0
```

Binary/installer upgrades replace the daemon and restart it; `data.db`,
`master.key` and configuration are untouched. History settings are
restart-scoped.

Android APKs are attached to this release as `android-v4.1.0`.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Full list of changes: [docs/CHANGELOG.md](CHANGELOG.md).
