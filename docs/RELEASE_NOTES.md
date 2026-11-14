# DigitDNS v5.8.11 — Android sinkhole status alignment

A patch release on top of v5.8.10. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Fixed

- **Android: the folded Custom Blocklist (Sinkhole) row now puts its Blocked pill
  in the right-hand status column.** It used to sit immediately after the
  chevron, on the left, while every domain row below carried its status pill on
  the right. The group row now lays the title/count out first and reserves the
  same gap + switch width as a domain row, so the Blocked pill lines up with the
  DIRECT/PROXY/BLOCKED pills it summarises. The pill width is shared with the row
  pills, so the column stays consistent at every font scale.

## Also in this line (v5.8.10)

- **Type scale.** The stylesheet gained a named text-size scale
  (`--text-2xxs` … `--text-2xl`, with line-height variants) and every hard-coded
  `font-size: Npx` was replaced with a token reference, so type adjustments are
  one-token changes rather than a sweep.
- **Semantic button intents.** `.btn-primary`, `.btn-ghost` and `.btn-soft` carry
  their own token-based colours and render correctly on dark, light and grey
  without per-theme overrides. They replace the brittle
  `[data-theme="grey"] button[class*="bg-cyan-500/"]` substring selectors.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.11` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.11/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.11
docker pull ghcr.io/digitrob/digitdns-agent:5.8.11
docker pull robmeister/digitdns:5.8.11
docker pull robmeister/digitdns-agent:5.8.11
```
