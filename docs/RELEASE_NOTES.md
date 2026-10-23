# DigitDNS v3.4.0

## Highlights

- **Android and web, finally in step.** This release is the biggest step yet
  toward full parity between the Android companion app and the web panel. The
  Clients screen, client details, server management, live query log, domain
  rules and the dashboard now behave the same way on a phone as they do in the
  browser — same actions, same confirmations, same look and feel. If you have
  been switching between the two, the differences you noticed are gone.
- **A complete Android companion app ships with this release.** The signed APKs
  for every architecture are attached to this same release page, alongside the
  server binaries and the offline bundle, so there is one place to download
  everything for a version. The app keeps its own version line, so a server
  update never forces an app update and vice versa.
- **Client management overhaul.** Adding and editing a subscriber now opens as a
  popup over the list instead of pushing a new screen; fields spread to use the
  available space without scrolling; quick actions (renew, lifetime, copy link,
  bot card) are aligned and fit on one row; swipe-to-delete with a confirmation
  dialog replaces the old per-row delete button.
- **Multi-server switching.** Save several servers on one phone, switch between
  them with one tap, and the app signs in automatically when it has saved
  credentials. A full server editor (name, host, port, admin path, HTTPS, and
  credentials) is built in.
- **Live Query Log polish.** Reload and pull-to-refresh are distinct again,
  pull-to-refresh re-anchors the feed and jumps to the top when new entries
  arrive, and the feed stays pinned to the latest queries while you read.
- **Web panel refresh.** The Clients page is now "Client Management"; the
  client details card reads "Registered IPs & CIDRs"; popup menus are glass-styled
  and anchored to their row; domain menus are centered on their domain.
- **API 3.1.** `expires_at` is now a first-class field on clients — set an
  absolute expiry or clear it with the zero timestamp. `api_version` in the
  version endpoint reports the contract generation.

## Android companion app

Side-load the APK for your device's CPU architecture onto Android 10 (API 29)
or newer. Verify the download against the matching `.sha256` before installing.

| APK | Architecture |
| --- | --- |
| `digitdns-arm64-v8a.apk` | 64-bit ARM (virtually all phones since ~2016) |
| `digitdns-armeabi-v7a.apk` | 32-bit ARM (older/low-end devices) |
| `digitdns-x86_64.apk` | x86-64 (emulators, some tablets) |
| `digitdns-universal.apk` | all of the above — larger, use if unsure |

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` pick everything up on
restart — no settings change, no migration. Binary installs: replace the
`digitdns` binary and restart the service. The Android app is a separate
download and a separate version line; install it only if you want the new
client-management and server-switching features.

Full changelog: docs/CHANGELOG.md

---

# DigitDNS v3.3.2

## Highlights

- **Fix: the banner's last line now reaches `docker logs` / Portainer.** The
  v3.3.1 wordmark printed without a terminating newline, so the container log
  driver held the final line (the fork attribution) in its buffer for the
  daemon's whole lifetime. The startup write is newline-terminated now — one
  code line and a regression test. Everything else from v3.3.1 carries over
  unchanged: DigitDNS wordmark, fork attribution line, and the boot summary
  showing reachable panel-domain addresses instead of the bind wildcard.

---

# DigitDNS v3.3.1

## Highlights

- **Branded startup banner.** The daemon log and console now open with the
  DigitDNS wordmark instead of the inherited HyperDNS art, and carry the fork
  attribution line, so journald / Portainer / docker logs state what the code
  is and where it is maintained.
- **Reachable addresses in the startup summary.** The boot footer no longer
  prints its listeners against the bind wildcard (`0.0.0.0`). Each line
  resolves the host: panel domain first, then the configured public IP, then
  the bind host for the SSH-tunnel case. The DoH and DoT lines honour a
  dedicated DoT domain when one is set, and the default DoH port is
  suppressed from the URL — the same rules the dashboard's subscription
  origin applies.
- Android companion: unchanged in this release (the app is a pure v3-API
  client; the banner lives in the Go daemon).

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` (or retagging `v3.3.1`)
pick the banner and summary up automatically on restart — no settings change,
no migration. Binary installs: replace the `digitdns` binary and restart the
service.

---

# DigitDNS v3.3.0

## Highlights

- **Android companion app.** A native Android client with full panel parity —
  login with session tokens and TOTP, dashboard stats, domain rules, clients,
  upstreams, a live query log with one-tap actions, diagnostics and settings.
  The signed APKs are attached to this same release (see below), with their own
  version line and an adaptive launcher icon.
- **Client management as popups.** Adding and editing a subscriber now opens
  over the list instead of pushing a new screen; the form is shared so the two
  paths cannot drift.
- **Swipe-to-delete with confirmation.** Rows delete on a swipe, but only after
  an explicit confirmation; a cancelled swipe snaps back. The old four-second
  undo toast is gone in favour of the confirmation.
- **Live Query Log polish.** Reload (no spinner) and pull-to-refresh (spinner)
  are distinct, mirroring the Domains page; the toolbar buttons keep fixed
  widths so toggling Pause/Resume no longer shifts Clear; buttons wrap instead
  of collapsing on narrow phones.
- **Dashboard stat cards.** Totals and secondary figures are right-aligned or
  centred so nothing is truncated at narrow widths.
- **TLS force renewal.** Request a real Let's Encrypt renewal even when a
  valid certificate is already on disk.
- **Upstreams API.** New `GET /api/v3/upstreams/stats` and
  `POST /api/v3/upstreams/delete`, matching the dashboard.
- **Stable live stream.** The panel now negotiates HTTP/1.1 only, so long-lived
  live-log streams are not reset by HTTP/2 intermediaries.

## Android companion app

Side-load the APK for your device's CPU architecture onto Android 10 (API 29)
or newer; the app carries its own version line, independent of the server's.

| APK | Architecture |
| --- | --- |
| `digitdns-arm64-v8a.apk` | 64-bit ARM (virtually all phones since ~2016) |
| `digitdns-armeabi-v7a.apk` | 32-bit ARM (older/low-end devices) |
| `digitdns-x86_64.apk` | x86-64 (emulators, some tablets) |
| `digitdns-universal.apk` | all of the above — larger, use if unsure |

Verify the download against the matching `.sha256` file before installing.

## Also in this release
- Dedicated pull-to-refresh indicators on Servers, TLS, About, Groups, Policies
  and the Live Log.
- Adaptive Android launcher icon that survives every OEM mask without clipping.
- Browser password-manager form fixes (hidden username fields).
- Numerous small UX fixes across the dashboard and the Android app.

Full changelog: docs/CHANGELOG.md
