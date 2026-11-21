# DigitDNS v5.9.8 — dashboard polish

This patch keeps the Live Traffic figures on one line when rates reach three
digits or more, and turns the DigitDNS brand mark into a shortcut back to the
dashboard on desktop and mobile. No database schema migration is required.
Existing settings, clients, blocklists, certificates, relay agents, egress
options and stored query history are preserved. Upstream selection defaults are
unchanged: Racing still maps to Hedged.

## Live Traffic keeps its headline intact

- **One-line in/out readings.** The card now uses a dedicated compact type
  scale instead of sharing the large single-number scale with the other tiles.
- **Compact values.** KB/s prints with one decimal below 100 and as a whole
  number from 100 upwards. At 1024 KB/s the card switches to MB/s, so neither
  direction needs a second line.
- **Exact values on hover.** The card title carries the unrounded inbound and
  outbound readings.

## Brand mark goes home

- **Desktop and mobile.** Clicking the sidebar or mobile header brand now opens
  the dashboard; both marks advertise the action with a title, ARIA label and
  keyboard focus ring.
- **Update button preserved.** The injected “Update available” button still
  opens its modal instead of also navigating away.

Source-contract tests pin the traffic row, unit spans, compact CSS scale, brand
links and home handler so a later edit cannot silently restore the wrap or drop
the shortcut.

## Android

The companion carries the matching `android-v5.9.8` version line (version code
578). The workflow's debug APKs are attached to this release alongside the
standard signed APKs for arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.8/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.8
docker pull ghcr.io/digitrob/digitdns-agent:5.9.8
docker pull robmeister/digitdns:5.9.8
docker pull robmeister/digitdns-agent:5.9.8
```
