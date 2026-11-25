# DigitDNS v5.11.7 — Configurable upstreams and honest latency

This release makes the resolver’s maintenance policy configurable from the panel and the Android app, coalesces identical cold lookups, and fixes a latency card that could report a median of 0.09 ms while the resolver was waiting tens of milliseconds on an upstream. No database schema migration is required.

## Advanced upstream settings

A new **Advanced upstream settings** card in Settings → Resolvers, and a matching screen under **Settings → Upstreams** in the app, now carry:

- **Bootstrap DNS** — plain resolvers used only to resolve DoT/DoH/DoQ hostnames.
- **Fallback DNS** — a last-resort pool tried only after every primary resolver failed. It is never used on the normal path.
- **Keep-warm scope** — off, active, active + next (default), or all encrypted upstreams.
- **Keep-warm interval** — 5–600 seconds; a save re-arms the loop immediately.
- **Probation** — how long a repeatedly failing resolver is skipped before it is probed again.
- **Startup cache warm** — pre-resolve the top 1–100 names from the last 24 hours in the background after a restart.

Everything applies on save except Bootstrap DNS, which needs a restart because the upstream dialers are built at startup. Defaults match the previous behaviour, so an existing install changes nothing until an operator edits the card.

## Faster cold paths

- **Single-flight misses.** Concurrent identical misses — same name, type, class and EDNS/DO/CD flags — now share one upstream query. Each client still receives its own transaction ID. A new `coalesced` counter shows how many queries joined an in-flight lookup.
- **Sibling prefetch.** When an allowed name misses, the resolver warms the other record types a browser asks for (A, AAAA, HTTPS/SVCB) in the background. The work is bounded to four workers and deduplicated, so a cold page load pays one upstream round trip instead of three.
- **Adaptive hedge mode.** The head start is now derived from the active resolver’s own median: balanced waits twice the median, aggressive waits one. Both are clamped and selectable in the advanced settings.
- **Live re-selection.** A resolver that stays roughly 25% slower than a healthy alternative for five consecutive checks is replaced immediately, without waiting for the next scheduled Adaptive run.
- **Keep-warm is narrower by default.** The background loop refreshes the active encrypted resolver plus the next fastest one, not every encrypted upstream. On a seven-resolver install this cut the maintenance traffic from seven queries every 20 seconds to two.
- **Startup cache warm** resolves the most popular names from history after the listeners are up, so a restart does not hand the first devices a burst of cold lookups.
- **Restart ordering.** The last-known resolver medians are cached on disk, so the first query after a restart starts with the order the previous run measured.

## Latency card corrected

The old “cache miss” figures included every query the cache did not answer: blocked names, custom records, self-service answers, quota refusals and proxy paths. Those finish in microseconds, so on a filtering resolver the median became the time to say “blocked” — 0.09 ms — rather than the time to resolve a name.

- The card now counts **only queries that reached an upstream**, including failed attempts.
- **p50, p95, p99 and Peak share the selected window.** Since-start values remain in the tooltips.
- The estimated p99 is capped at the exact window peak, so it can no longer display above Peak.
- The history **Avg latency** card uses a resolution-only counter. Ranges containing pre-upgrade buckets are marked `≈` until those buckets age out.

## Range chips

The Resolver Latency card now has **5m / 15m / 1h / 6h / 24h** chips on web and Android. The server keeps 60 one-minute slots and 288 five-minute slots, so every range is computed from real observations rather than a rescaling of the five-minute figures.

## Force

**Force** now runs a full benchmark and selects the fastest healthy resolver from that run, immediately, instead of waiting for the configured interval. If a scheduled benchmark is already running, Force waits for it and uses its results rather than starting a second probe.

## Web layout

The **Force** button now uses the same 32 px box and padding as the interval chips. It sits next to the chips and wraps with them, so it never clips an interval label.

Ranked history tables are fixed at ten rows again; the `10 / 50 / all` toolbar toggle has been removed.

## Verification

Full Go suite, `go vet`, the race detector, 112 dashboard tests, Android unit tests and detekt pass. The release was exercised on the live Norwich instance: startup cache warm resolved 100/100 names in about 1.1 seconds, keep-warm stayed at two encrypted connections, and cached queries returned in 0–1 ms.

The Android companion moves to **android-v5.11.7**, version code **589**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.7/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.7
docker pull ghcr.io/digitrob/digitdns-agent:5.11.7
docker pull robmeister/digitdns:5.11.7
docker pull robmeister/digitdns-agent:5.11.7
```
