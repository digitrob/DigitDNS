# DigitDNS v5.6.0 — egress options, engine switch and system log

A feature release on top of v5.5.7. No database schema migration is required;
existing settings, clients, blocklists, certificates and relay agents are
preserved.

## Egress Options

Relay Agents is now **Egress Options**: one page for every path the hub can use.

- **Relay Agents** remain public exit points clients connect to directly.
- **SOCKS5** lets the hub dial the origin through a proxy. Remote DNS is the
  default, with optional local resolution; the destination still passes the
  hub's SSRF guard first.
- **WireGuard** runs userspace `wireguard-go` + gVisor netstack. Paste a
  standard `wg-quick` configuration or upload a `.conf` file.
- **Fail-closed** is the default. An offline or disabled fail-closed egress
  produces SERVFAIL instead of silently falling back to this server.
- Health, latency, traffic, last error and fail-closed state are visible per
  connection.
- SOCKS5 passwords and WireGuard private keys are write-only: the API reports
  only whether they are set.
- Route via is grouped into **This Server / Relay Agents / SOCKS5 / WireGuard**.
  New assignments use `egress:<id>`; existing agent IDs are unchanged.
- API: `/api/v3/egresses` with create, update, delete, enable/disable and test.

## Engine switch

- A global **Engine ON/OFF** control is available from the panel and Android.
- **Off** bypasses custom records, the rule matcher, service/agent assignments,
  proxying and ad blocking. Queries resolve directly through the upstream pool.
- Access control and traffic quota remain enforced.
- New SNI relay connections are refused while the engine is off.
- The choice is persisted and applied live.

## System log

- The daemon keeps a bounded in-memory tail of its own log.
- New **System Log** page in the web sidebar and Android Settings.
- Copy all / export `.log` on web; copy and save on Android.
- Web and Android refresh immediately when opened and keep updating while
  visible.

## Upstream resolvers

- **Force** now selects the absolute fastest healthy resolver, bypassing the
  normal 12% hysteresis. It stays selected until the interval expires or Force
  is pressed again.
- The selected resolver carries a clear **FASTEST** badge.
- Top upstreams now shows provider logos and a **By Provider** toggle that
  aggregates the ranking by DNS provider.

## Dashboard and shell

- Metric headline values scale with the card's own width; narrow cards keep the
  readable size and wide cards grow, without overflow.
- Live Traffic upload/download values match the other cards and align with their
  values.
- The sidebar can be folded to an icon rail; the choice is remembered.
- Theme, engine and sidebar controls use the existing bottom action rows without
  adding another row.

## Android

- Egress Options with Relay Agents, SOCKS5 and WireGuard subcards, add/edit
  dialogs, `.conf` upload, health/traffic/fail-closed summary and grouped relay
  picker.
- Global engine control in the bottom bar / navigation rail.
- System Log section with copy and save.
- Live Log action filters are coloured status chips instead of a popup.
- Top upstreams shows provider logos and a By Provider toggle.
- Egress summary stays on one row.

## API

- `/api/v3/egresses` — CRUD, enable/disable, test.
- `/api/v3/engine` — read and set the global engine state.
- `/api/v3/system-log` — bounded log tail.
- `/api/settings/resolution/run-adaptive` now performs a forced selection.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.6.0` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.6.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.6.0
docker pull ghcr.io/digitrob/digitdns-agent:5.6.0
docker pull robmeister/digitdns:5.6.0
docker pull robmeister/digitdns-agent:5.6.0
```
