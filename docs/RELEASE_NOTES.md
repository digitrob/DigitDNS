# DigitDNS v5.5.1 — DoQ completion and installer sync

A small, server-focused patch on top of v5.5.0. No resolver, API or database
schema changes are required. Existing settings, clients, blocklists and
certificates are preserved.

## Fixed

- **DNS-over-QUIC now sends STREAM FIN after the query**, as RFC 9250 §4.2
  requires. Strict public DoQ resolvers were waiting for the finished message,
  which made a DoQ upstream show no latency and look unmeasured. AdGuard and
  Quad9 DoQ endpoints now probe and report latency normally.
- **The offline bundle installer now opens DNSCrypt TCP/UDP 5443** in its
  firewall rules, matching the online installer.

## Android

The companion app is **5.5.1 (542)**. It carries only the version bump; its
behaviour is unchanged from 5.5.0.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.1
docker pull ghcr.io/digitrob/digitdns-agent:5.5.1
docker pull robmeister/digitdns:5.5.1
docker pull robmeister/digitdns-agent:5.5.1
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.1/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.5.1-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.1` tag.
