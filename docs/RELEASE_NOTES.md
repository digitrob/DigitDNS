# DigitDNS v5.0.2 — Allowed-only Top domains and polish fixes

A small, compatibility-safe patch over **5.0.1**. No database migration and no
configuration changes; upgrade in place.

## 📊 Top domains vs Top blocked

- **Blocked names no longer appear in Top domains.** They are counted only in
  **Top blocked**.
- The correction runs at read time, so existing 24h/7d/30d/90d buckets are fixed
  immediately — no need to delete `stats.db` or wait for old data to age out.
- `Other` is clamped so the subtraction cannot underflow.
- The ranked entries are exact. Both underlying tables are bounded top-N, so the
  evicted tail is approximate; this is documented in the API notes.

## 📱 Android

- **Bulk import for DNS Rewrites** now reads and parses files off the UI thread,
  with localized “Reading…”, “Importing…” and file-error messages.
- The bulk-import summary is localized and shows added/updated/unchanged/invalid
  counts.
- The compact pause/resume switch exposes one accessible switch control instead
  of a wrapper plus an inner switch.
- **Queries by Relay** wraps to two lines on 320dp phones instead of truncating;
  its row height matches the adjacent Protocols card.
- Locale-safe uppercase handling for card headings, protocol names, summary
  labels and action filters (Turkish `i` no longer becomes `İ`).
- Search normalization in Clients, Live Log and Ad Blocking uses `Locale.ROOT`.

## 🪵 Live Log

- **Rewrite** queries now appear under the **Direct** filter, matching their
  local-override behaviour.
- The Rewrite badge, query count placement and card titles keep their 5.0.1
  appearance.

## 🧹 Cleanup

- Removed a real-looking test host and subscription token from the Android test
  fixture.
- Added an aggregate regression test for the blocked-domain tail clamp.
- Release workflow now sets a descriptive release title automatically.
- README screenshots refreshed for the current dashboard, Queries by Relay and
  DNS Rewrites UI.

## ⬆️ Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:5.0.2
docker pull ghcr.io/digitrob/digitdns-agent:5.0.2
```

Recreate the container as usual. Android APKs are attached as
`android-v5.0.2` (arm64-v8a, armeabi-v7a, x86_64 and universal).

## 📦 Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Screenshots: [README gallery](https://github.com/digitrob/DigitDNS#screenshots).
Full changes: [docs/CHANGELOG.md](https://github.com/digitrob/DigitDNS/blob/v5.0.2/docs/CHANGELOG.md).
