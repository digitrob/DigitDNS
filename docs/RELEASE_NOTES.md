# DigitDNS v5.6.1 — engine switch and shell fixes

A patch release on top of v5.6.0. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Fixed

- **Android engine switch is no longer one-way.** Turning the policy engine off
  and back on now works. The enable request previously serialised to an empty
  body because `enabled: true` was treated as a default value and omitted; the
  server rejected it. Turning it off sent `false` and worked.
- **Android engine switch colours.** The switch track, thumb and border are
  green when the engine is on and red when it is off.
- **Folded web sidebar scrolling.** Only the navigation list scrolls. The bottom
  controls — public IP, diagnostics, restart, flush, theme, sign out, engine and
  collapse — stay fixed. Folding the sidebar no longer creates a stray vertical
  scrollbar.
- **Web System Log copy.** Copy is now a markdown-style icon button inside the
  log window, top-right, instead of a header button.
- **Upstream Force spacing.** The Force control sits closer to the interval
  chips and no longer floats at the far edge of the row.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.6.1` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.6.1/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.6.1
docker pull ghcr.io/digitrob/digitdns-agent:5.6.1
docker pull robmeister/digitdns:5.6.1
docker pull robmeister/digitdns-agent:5.6.1
```
