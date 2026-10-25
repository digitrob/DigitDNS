# DigitDNS v3.4.8

## Highlights

- **Clear finally means clear.** The live log's Clear button now empties the
  server's own recent-query buffer, not just the page you are looking at. Every
  other open dashboard is told to drop its rows too, and a reconnect no longer
  brings the cleared queries back. If the server cannot be reached, the panel
  says so instead of pretending it worked.
- **Pause pauses, and nothing else.** While the log is paused, arriving queries
  are kept in the background instead of being thrown away, so Resume shows what
  happened in the gap. A reconnect can no longer repaint the table while you
  have it frozen.
- **The log stays a hundred queries.** The browser keeps exactly as many rows as
  the server does, so a long-running tab cannot grow without bound and a refresh
  never shows more than the server would replay.
- **Rows are easier to track.** The Proxied Domains table and the live log now
  alternate a faint row tint. Paused domains are washed amber instead of being
  faded out, which used to dim the switch and the status badge you need to turn
  them back on.
- **A smaller Android dashboard.** All six metric tiles are roughly half the
  height they used to be: smaller numbers, a tighter text step that matches the
  rest of the app, less padding, and Live Traffic's `Total` on the same line as
  the up/down rates. Every card in a row still lines up, and nothing wraps or
  gets clipped.
- **The Android server box does more.** Swipe the address sideways to blur it
  for privacy, or up/down to swap between the public IP and the domain the
  server answers on. The flag and copy icon stay exactly where they were.

## Under the hood

- New `POST /api/stream/clear`; the Clear button uses it, and a `cleared` event
  keeps other panels in step.
- The dev container's health check now asks over HTTPS when the panel runs
  TLS, instead of reporting a healthy container as sick.

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` pick everything up on
restart with no migration. Binary installs: replace the `digitdns` binary and
restart. The Android app is a separate download on this page — version 3.4.8
(versionCode 348) — and updates independently of the server.

Full changelog: docs/CHANGELOG.md

---

# DigitDNS v3.4.7

## Highlights

- **Your server now wears its country.** A flat flag sits beside the server's
  public IP in the web panel and on the Android dashboard. It comes from a
  GeoLite2 database the daemon keeps on disk and refreshes daily, so the flag
  is correct without any per-lookup calls leaving your server.
- **Refresh that database whenever you like.** The Server Location card (web:
  Settings & SSL, Android: About) shows the country, the database date and an
  **Update database** button. No account or key is needed — it uses a free
  public GeoLite2 mirror.
- **Update the server from the panel.** The web's Settings & SSL page now has a
  **Server Update** card beside Server Location: check for a release and, when
  one exists, run the same verified update the sidebar badge does — download,
  SHA-256 check, backup, restart and progress. On Android the About page's
  update card got the same shape, with a refresh icon on **Check for updates**
  and buttons that line up with the Server Location action.
- **Small polish.** The flag is centred against the whole IP block and clear of
  the copy icon, dates read DD.MM.YYYY, and the Android About page keeps its
  cards on one screen.

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` pick everything up on
restart — no setting changes, no migration. Binary installs: replace the
`digitdns` binary and restart. The GeoIP database downloads shortly after
startup; until then the flag is simply absent. The Android app is a separate
download and version line — this release's APKs are on this page as usual.

Full changelog: docs/CHANGELOG.md

---

## Highlights

- **Certificate renewals now live together on SSL/TLS.** "Certificate Auto-renew"
  sits between "Issue a certificate" and "DoT/DoH Auto-renew", with the line
  *Renew Let's Encrypt SSL certificate.* Nothing about renewal changed — the
  switch simply moved off the Subscription page, which makes the certificate
  controls one place instead of two.
- **The Subscription page is readable again.** Its two switches used to be drawn
  on top of each other, producing the garbled "Auto-renew certificate /
  Subscription portal" text you may have seen. With one switch left there, the
  card is a clean single row.
- **A faster Proxied Domains page on the web.** Sort, the domain count, a search
  box and the bulk Pause/Resume/Remove buttons share one row, and the list
  narrows as you type — the same behaviour as the Live Query Log. Select-all
  follows the filter, so a search can be acted on as a group.
- **Autocomplete on Add.** The Add field suggests names that are already listed
  and the table narrows while you type, which makes duplicates obvious before
  they are saved. Adding one anyway now tells you it is already there.
- **Small dashboard polish on Android.** The server-switch icon now lines up with
  the ONLINE badge, and both certificate-renewal switches read their real state
  from the server instead of assuming "on".

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` pick everything up on
restart — no settings change, no migration. Binary installs: replace the
`digitdns` binary and restart the service. The Android app is a separate download
and version line; this release's APKs are on this page as usual.

Full changelog: docs/CHANGELOG.md

---

## Highlights

- **A calmer, more predictable dashboard.** Upstream Racers now leads the
  Android dashboard, the six metric tiles are all the same size, and the order
  has a one-tap reset in Settings. Scrolling the dashboard can no longer pop an
  accidental "remove upstream" prompt.
- **Server management that fits the screen.** The Manage Servers dialog keeps
  three saved servers visible, scrolls only the list when there are more, and
  keeps Add server, the signed-in details, Sign out, Cancel and Done reachable.
  The Add-server form has a Cancel that returns to the app.
- **The Domains tab no longer crashes** on installs whose proxied list carried a
  duplicated domain: duplicates are ignored on load and on save.
- **Client editing, finished.** The edit-client card has a roomier IP Addresses
  box and a compact Expiry; Save always persists every field, closes the card,
  and warns when the device limit trimmed host addresses. Cards with long IP
  lists stay a normal size.
- **Cleaner proxied list.** The web table uses the available width (domain left,
  actions and status compact on the right) with no Date Added column, and the
  Android rows are a single aligned line.

## Upgrading

Docker users pulling `ghcr.io/digitrob/digitdns:latest` pick everything up on
restart — no settings change, no migration. Binary installs: replace the
`digitdns` binary and restart the service. The Android app is a separate
download and version line; install it only if you want these fixes.

Full changelog: docs/CHANGELOG.md

---

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
