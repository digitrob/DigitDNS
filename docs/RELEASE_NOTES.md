# DigitDNS v3.3.0

## Highlights

- **Android companion app.** A native Android client with full panel parity —
  login with session tokens and TOTP, dashboard stats, domain rules, clients,
  upstreams, a live query log with one-tap actions, diagnostics and settings.
  Ships as signed multi-ABI APKs (`digitdns-<abi>.apk`) on the Android release
  page, with its own version line and an adaptive launcher icon.
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

## Also in this release
- Dedicated pull-to-refresh indicators on Servers, TLS, About, Groups, Policies
  and the Live Log.
- Adaptive Android launcher icon that survives every OEM mask without clipping.
- Browser password-manager form fixes (hidden username fields).
- Numerous small UX fixes across the dashboard and the Android app.

Full changelog: docs/CHANGELOG.md
