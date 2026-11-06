# DigitDNS v5.2.1 — proxy leak protection and refresh reliability

**A patch release.** 5.2.1 closes the `HTTPS`/`SVCB` leak on proxied domains,
adds a live Proxy Leak Protection control to the web panel and Android app, and
fixes several refresh and live-log issues found while rolling it out.

There are no database schema changes and no breaking API changes. The new
`proxy_leak_protection` setting defaults to `true`; existing proxied domains
start withholding `HTTPS`/`SVCB` answers after the upgrade.

## Proxy leak protection

- Proxied names no longer receive upstream `HTTPS`/`SVCB` answers. The hub
  answers `NOERROR` with no answers, no `AD` and no cache entry, so clients
  cannot discover HTTP/3 or the origin's real IPv4/IPv6 hints and bypass the
  SNI relay.
- Direct names keep their normal `HTTPS`/`SVCB` records; blocked names already
  return the block response for every query type.
- New live `proxy_leak_protection` setting, default `true`. Changing it applies
  to the running resolver without a restart. Turning it off requires an
  explicit confirmation.
- New authenticated routes:
  - `GET/POST /api/settings/proxy-leak`
  - `GET/POST /api/v3/settings/proxy-leak` for Android and API-key clients.

## Web panel

- **Settings → Proxy Leak Protection** now sits beside **DNSSEC Validation**.
  The two cards share a responsive row, stack on mobile, and the built-in root
  anchors stay visible in a fixed-height block.
- The DNSSEC and proxy switches stay hidden until their saved state arrives, so
  a page refresh no longer animates them from a default position.
- Signing in now reloads the dashboard document instead of reusing old markup.
  An upgraded panel shows the new UI, settings and data immediately; the
  standalone login redirect also bypasses the back/forward cache.
- Live-log `PROXY · QUIC FALLBACK` rows are treated as proxied queries: the
  **PROXY** filter, RELAY column and aggregate proxied totals now include them.

## Android companion

- The Settings section is now **DNSSEC/Proxy Leak**.
- New **Proxy Leak Protection** card below the DNSSEC card, with `ACTIVE`/`OFF`
  status, a live switch, a confirmation before disabling, warning/off notes and
  compact `HTTPS/SVCB`, `h3/QUIC` and `IPv4/IPv6` fact tiles.
- Both cards fit the Settings area without scrolling on a standard phone.
- Pull-to-refresh no longer disables the **Save DNSSEC settings** button or
  clears the saved-status line.
- Added API, ViewModel, repository and routing tests for the live proxy guard.

## Upgrade

The in-panel self-updater and the one-line installer use the same release.

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
docker pull ghcr.io/digitrob/digitdns:5.2.1
docker pull robmeister/digitdns:5.2.1
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.1` tag.
