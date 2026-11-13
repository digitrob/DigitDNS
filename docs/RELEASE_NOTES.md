# DigitDNS v5.8.1 — alert pictograms and resolver hot-path

A minor release on top of v5.8.0. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **Alert channel pictograms.** Web and Android alert channel rows show a
  destination pictogram (ntfy, Telegram, Discord, Slack, Gotify, Webhook) in the
  kind picker and on the card, with a check on the active choice. The web
  picker is a custom menu so the glyphs render in dark, grey and light themes.
- **Android alert channel cards.** Each channel is its own bordered tile: the
  kind pictogram opens the type menu, "Enabled" is labelled, and the channel
  name gets its own full-width row so it can never collapse on a phone.
- **Empty state.** "No channels yet — add one to receive alerts." on both
  clients when there are no channels.
- **Benchmarks.** `BenchmarkOrderedByLatencyWarm`, `BenchmarkRecordStat` and
  `BenchmarkQueryLogPushDisabled` in the upstream and service packages.

## Changed

- **Resolver hot path.** Upstream stats are published through an
  `atomic.Pointer` so `recordStat` and `upstreamAvailable` no longer take the
  pool `RLock` on every answer; the cached latency order is an immutable
  snapshot read with a single atomic load (Hedged/Best Known). The query-log
  service caches its settings, so `Push` no longer reads (and decrypts) the
  setting from bbolt on every DNS query — the default, with logging off, is
  8.5 ns/op, 0 allocs.
- **Dashboard 1 Hz polish.** `renderCacheHealth`, `renderProxyDrops` and the
  latency badge compare before writing, and the resolver list and QPS chart are
  skipped while another tab is visible — the poll keeps running, only the paint
  is deferred.
- **Transport palette sharing.** The Android resolver chips resolve their colour
  and label from the theme's `ProtocolPalette` (`transportColor` /
  `transportLabelRes`) instead of a second local table, matching the live log
  and history cards.

## Fixed

- **Android detekt.** `TransportChips` no longer exceeds the
  cyclomatic-complexity threshold (the transport colour/label lookup moved to
  the theme).
- **Android alert channel name collapse.** The `weight(1f)` field now has a
  real row to fill instead of being squeezed beside the kind selector, switch
  and delete button.
- **Web alert type picker.** The native `<select>` (which cannot render icons)
  was replaced with a pictogram menu.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.1` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.1/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.1
docker pull ghcr.io/digitrob/digitdns-agent:5.8.1
docker pull robmeister/digitdns:5.8.1
docker pull robmeister/digitdns-agent:5.8.1
```
