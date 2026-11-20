# DigitDNS v5.9.4 — a lighter DNS hot path and a steadier live log

This patch keeps the v5.9.3 feature set and focuses on responsiveness: cache hits
allocate less, the live-log ring is safe under concurrent writers, and the
once-a-second resolver ranking rebuild no longer takes a dashboard-sized detour
on a query path. The Ad Blocking editor also stops background polls from
overwriting text while an operator is typing.

No database schema migration is required. Existing settings, clients,
blocklists, certificates, relay agents, egress options and stored query history
are preserved.

## Faster cache hits

- **The dead message-pool call is gone.** Every cache hit called `msgPool.Get()`
  and immediately overwrote the result with `entry.msg.Copy()`, so the pool stayed
  empty and the hit paid for two message allocations. A hit now allocates only
  the reply copy.
- **Answer sections are rendered only when something will read them.** While no
  live-log stream is open and persistent query history is disabled, the query
  still records its latency, action, rule and result code, but no longer
  stringifies every answer record for a log item that would be discarded.
- **The client address is parsed once.** Common UDP and TCP peers are read
  straight from their address structs instead of being formatted to text,
  split, and formatted again on every query.

On the local benchmark these changes took a cache lookup from about 880 ns and
7 allocations to about 330 ns and 6 allocations.

## A live log that cannot tear under load

- **`PushQueryLog` now claims a sequence number, then publishes one immutable
  entry pointer.** The previous ring wrote a slot before its compare-and-swap,
  so a losing writer could overwrite an already published row and readers could
  observe partly updated strings and slices.
- **Clear advances a floor instead of zeroing slots under a writer.** A
  reconnecting live-log stream can no longer replay queries from before a
  clear, and the ring remains clean under the race detector.

## Resolver ranking without the dashboard detour

- **The once-a-second latency-order rebuild reads only the latency value it
  needs.** It no longer calls the full `GetUpstreamStats()` path, which copied
  every health counter, formatted timestamps, built a dashboard-sized slice and
  sorted it on whichever cache miss crossed the rebuild tick.
- **Upstream selection defaults are unchanged.** Racing still maps to Hedged;
  the adaptive and force paths keep the same rules and the same resolver-card
  numbers.

On the local benchmark the cold rebuild fell from about 1148 ns / 1840 B / 8
allocations to about 450 ns / 432 B / 5 allocations. Dashboard statistics and
per-query stats recording are unchanged.

## Dashboard editing

- **Background polls no longer overwrite the Ad Blocking allowlist or custom
  rules while they are being edited.** Focused and unsaved fields win; a
  successful save rebases the tracking on what was written, so later polls can
  still reflect changes from another panel session.

## Android

The companion carries the matching `android-v5.9.4` version line. The workflow's
debug APKs are attached to this release alongside the standard signed APKs for
arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.4/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.4
docker pull ghcr.io/digitrob/digitdns-agent:5.9.4
docker pull robmeister/digitdns:5.9.4
docker pull robmeister/digitdns-agent:5.9.4
```
