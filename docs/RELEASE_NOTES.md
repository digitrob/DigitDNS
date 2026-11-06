# DigitDNS v5.1.2 — Backend performance and telemetry fast path

**A performance release.** 5.1.2 reduces the work the resolver does around every
answer, keeps the live log from becoming a bottleneck, and makes large policy
sets cheaper to evaluate. There are no schema, API or configuration changes.

## Resolver

- **Typed DNS answers.** Proxy A/AAAA, blocked null-IP, custom-record and
  self-name answers no longer format and parse a DNS record on every query.
  Public addresses and relay agent IPv4s are parsed once when configuration
  changes.
- **Compiled policy indexes.** Per-domain upstream profiles are compiled when
  the policy is installed. Relay assignments use exact and wildcard indexes
  instead of scanning every assignment on every proxied query.
- **Narrower cache invalidation.** Saving a Direct/Proxy/Block rule no longer
  clears the whole DNS cache. The cache is flushed only when a resolution
  profile actually changes; the matcher and custom-record checks run before the
  cache, so a normal rule edit cannot be bypassed by an old entry.

## Live log and telemetry

- When no dashboard is connected, the resolver no longer JSON-encodes each
  query or walks the SSE subscriber map.
- The 100-entry history is a fixed ring, so it no longer reallocates and copies
  the window periodically.
- Frames are batched for up to 20 ms or 64 events and flushed once per batch,
  instead of one write and one flush per query.

## Measured locally

Handler-level A/B, same machine and no live-log subscriber:

| Path | Before | After |
|---|---:|---:|
| Proxy answer | ~3.9 µs, 25 allocs | ~0.6 µs, 5 allocs |
| Blocked answer | ~3.6 µs, 24 allocs | ~0.5 µs, 5 allocs |
| Cache hit | ~2.1 µs, 11 allocs | ~0.65 µs, 7 allocs |

Separate microbenchmarks:

- Profile lookup with 200 profiles: ~41.8 µs → ~0.1 µs.
- A-record construction: ~1.77 µs, 15 allocs → ~39 ns, 1 alloc.

Loopback UDP throughput is dominated by the local network stack, so end-to-end
QPS improved only a few percent in that test. The CPU and allocation reductions
are the durable gain, especially at higher query rates.

## Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

If you pin a version:

```bash
docker pull ghcr.io/digitrob/digitdns:5.1.2
docker pull ghcr.io/digitrob/digitdns-agent:5.1.2
```

The in-panel self-updater and the one-line installer use the same release.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.1.2` tag.
