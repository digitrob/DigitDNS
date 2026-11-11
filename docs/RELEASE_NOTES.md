# DigitDNS v5.5.2 — trustworthy upstream latency

A server and panel patch on top of v5.5.1. No resolver, API or database schema
migration is required. Existing settings, clients, blocklists and certificates
are preserved.

## Upstream measurement

- **Adding a resolver now benchmarks it automatically.** Both the web and
  Android add endpoints start an asynchronous single-resolver probe, so the new
  entry can show a latency on the next stats poll.
- **Benchmarks discard a cold warm-up and report the median of three measured
  queries.** This removes the TCP/TLS/QUIC/HTTP handshake from the displayed
  figure that previously made a fresh benchmark look much slower than a re-run.
- **A benchmark needs a majority of its measured probes to answer**, so one
  lucky packet cannot mark a failing resolver healthy.
- **Live p50 is used only after three successful live exchanges.** Before that
  the card shows the stable warm-benchmark median and marks it as a probe value.
  One slow live query can no longer dominate the card.
- The upstream stats API adds `latency_is_probe` for clients that want to show
  the measurement source.

## Web upstream card

- Clicking a checked provider server removes it from the pool and card
  immediately, with an optimistic pending state and a stale-poll guard.
- The upstream field no longer invites browser postal-address autofill.
- Provider dropdown type is larger and the card heading aligns with the chart
  card.
- Typing a bare provider name such as `quad9` now asks the operator to choose
  one of that provider’s servers instead of creating `quad9:53`.

## Android

The companion app is **5.5.2 (543)**. It carries the `latency_is_probe` field
and the version bump; upstream-add behaviour is unchanged.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.2
docker pull ghcr.io/digitrob/digitdns-agent:5.5.2
docker pull robmeister/digitdns:5.5.2
docker pull robmeister/digitdns-agent:5.5.2
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.2/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.5.2-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.2` tag.
