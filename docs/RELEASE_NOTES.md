# DigitDNS v5.1.4 — Cleaner Top Domains and Docker Hub

**A maintenance release.** 5.1.4 removes reverse-DNS noise from the ranked
domain tables and publishes the container images to Docker Hub alongside GHCR.

There are no schema, API or configuration changes.

## Top Domains

- `in-addr.arpa` and `ip6.arpa` names are excluded from **Top Domains** and
  **Top Blocked Domains**.
- Reverse names still count toward total queries, blocked/cached/failure
  counters, **ByQType** and the live query log, so PTR volume remains visible on
  the qtype card.
- Existing `stats.db` buckets are filtered on read, so historical reverse rows
  disappear immediately rather than waiting for retention.

## Container registries

The hub and relay-agent images are now published to both registries with the
same tags, multi-arch manifests and cosign signatures:

- `ghcr.io/digitrob/digitdns`
- `ghcr.io/digitrob/digitdns-agent`
- `robmeister/digitdns`
- `robmeister/digitdns-agent`

## Upgrade

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
docker pull ghcr.io/digitrob/digitdns:5.1.4
docker pull robmeister/digitdns:5.1.4
```

The in-panel self-updater and the one-line installer use the same release.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.1.4` tag.
