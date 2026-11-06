# DigitDNS v5.1.1 — Sky-blue Proxy status and grey favicon

**A small Graphite polish release.** 5.1.1 gives the Proxy route one restrained
colour in the neutral theme instead of leaving it grey, and finishes the grey
favicon work on the web.

This is a drop-in upgrade. There are no schema, API or configuration changes.

## Web panel

- **Proxy status is sky blue** (`#7DD3FC`) in the Live Log and the Proxied
  Domains list.
- Proxy domain names use a semantic class rather than the shared `text-cyan-300`
  utility, so other cyan text is unaffected.
- Direct stays green, Blocked red and Paused grey. The rest of Graphite remains
  monochrome, with no glow on the Proxy badge.
- The whitelist-mode chip stays neutral even though it shares the
  `.badge-proxy` shape.
- **Grey favicon** in the dashboard and standalone login documents.

## Android companion

- **Proxy pill and live-log badge are sky blue** in Graphite.
- A dedicated `ExtendedColors.proxy` token keeps the accent separate from the
  neutral `info` colour, so TCP and informational accents remain grey.
- Direct, Blocked and Paused colours are unchanged.

## Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

If you pin a version:

```bash
docker pull ghcr.io/digitrob/digitdns:5.1.1
docker pull ghcr.io/digitrob/digitdns-agent:5.1.1
```

The in-panel self-updater and the one-line installer use the same release.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.1.1` tag.
