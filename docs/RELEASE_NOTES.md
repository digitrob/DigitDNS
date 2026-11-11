# DigitDNS v5.5.3 — quieter panel and faster agent recovery

A maintenance release on top of v5.5.2. No resolver, API or database schema
migration is required. Existing settings, clients, blocklists and certificates
are preserved.

## Panel

- **Removed the global “upstream resolvers degraded” banner.** A single failed
  DoH3 probe could push a page-wide warning across every tab and break the
  layout. Per-resolver health remains on the Upstream Resolvers card through the
  dot, latency badge and tooltip.
- The transport-listener warning remains: if DoH3/DoQ is enabled but not
  listening, that is a real transport failure and still appears.
- Removed the unused 2.8 MB legacy Tailwind stylesheet from the source tree and
  Docker context.
- Removed stale v2.x version headings from visible documentation.

## Agent ↔ hub

- **Heartbeat recovery is capped at one minute.** Catalogue sync still backs off
  to five minutes because the cached allowlist keeps serving, but an agent now
  reports back within a minute after a long hub outage.
- **HTTP/2 keepalive** pings idle agent-hub connections and closes a half-dead
  one before the request timeout.
- **GOAWAY is treated as a transport drop**, so the catalogue long-poll is
  re-armed immediately after a hub restart or connection drain.

## Android

- The live-query stream now waits on a completion signal from `onClosed`,
  `reconnect()` or `teardown()` instead of polling a flag every 500 ms.
- App version **5.5.3 (544)**.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.3
docker pull ghcr.io/digitrob/digitdns-agent:5.5.3
docker pull robmeister/digitdns:5.5.3
docker pull robmeister/digitdns-agent:5.5.3
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.3/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.5.3-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.3` tag.
