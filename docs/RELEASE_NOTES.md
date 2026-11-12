# DigitDNS v5.7.0 — full-instance relay mode and unified egress cards

A minor release on top of v5.6.1. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **Full-instance relay mode.** A full DigitDNS instance can act as a relay for
  another hub while continuing to serve its own clients. Relay traffic and local
  proxying share the same 80/443 listeners. Hub clients only reach hosts the hub
  approves; local clients keep their existing behaviour.
- **Relay mode card** on web and Android with the hub URL (including the hidden
  admin path), a write-only agent key, first-time enrolment token, online and
  sync state, hosts, clients, relay traffic and a real **Test connection** that
  fetches the catalogue and reports ping.
- **Cached catalogue.** A hub outage no longer stops relay service; the last
  synced catalogue keeps working until the hub returns.
- **Separate relay counters.** Relay bytes are reported independently of local
  proxy traffic.
- **API.** `GET/POST /api/v3/relay-mode` and `POST /api/v3/relay-mode/test`,
  mirrored on the web API and documented in the v3 OpenAPI spec.

## Changed

- **SOCKS5 and WireGuard cards match relay agents.** Status beside the name,
  compact header actions, one traffic/last-seen meta line, a colour-coded ping
  badge, **Test connection** in the footer, and ten-second auto-refresh.
- **Relay card footer.** The agent number is pinned to the far right and the ping
  badge has a fixed slot, so text no longer jumps while a test runs. The separate
  refresh button was removed.
- **Android.** The engine ON/OFF control appears only on the Dashboard. The
  upstream **FASTEST** badge now sits beside the hits/errors count.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.7.0` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.7.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.7.0
docker pull ghcr.io/digitrob/digitdns-agent:5.7.0
docker pull robmeister/digitdns:5.7.0
docker pull robmeister/digitdns-agent:5.7.0
```
