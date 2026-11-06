# DigitDNS v5.1.3 — Faster page boot and Android UI

**A frontend responsiveness release.** 5.1.3 removes the dashboard flash when a
non-dashboard route is reloaded, reduces the work the Android app does while a
tab is off screen, and batches the live query feed so high traffic cannot
overwhelm Compose.

There are no schema, API or configuration changes.

## Web panel

- **The reloaded tab is the first tab painted.** A pre-paint route hint and CSS
  select the tab from the URL before `app.js` runs, so reloading `/clients` or
  `/logs` no longer shows Dashboard and then swaps.
- **Only the active tab loads at boot.** The Proxied, Clients, Agents, Filters,
  Live Log and Settings data is fetched when that tab is selected.
- **Parallel boot probes.** `/api/auth/me` and `/api/config` now run together,
  saving one round trip.
- **Config failures keep the requested tab** instead of falling back to
  Dashboard.
- The Dashboard nav highlight is neutralised during boot, so the wrong tab is
  never briefly marked active.

## Android companion

- **Faster cold-start path.** Credential migration, DataStore reads and
  Keystore-backed secret reads run off the main thread.
- **No navigation block on session refresh.** The shell opens from cached
  credentials; an optimistic session refresh runs in the background.
- **Lifecycle-aware work.** Dashboard polling, history refresh and the shared
  live-query stream stop when the tab is not visible.
- **Live log batching.** UI snapshots are published at most four times per
  second instead of rebuilding the 200-entry buffer for every query. Alerts
  still receive every event immediately.
- **Less list recomposition.** Domain rows, filtered clients and visible log
  rows are computed once per relevant state change.
- **Smoother live log scrolling.** A new query no longer starts a scroll
  animation when the reader is away from the top.
- **Clients render sooner.** The list paints as soon as the client call
  returns; link origin and public address fill in concurrently.
- History top tables now request the five rows the phone displays.

## Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

If you pin a version:

```bash
docker pull ghcr.io/digitrob/digitdns:5.1.3
docker pull ghcr.io/digitrob/digitdns-agent:5.1.3
```

The in-panel self-updater and the one-line installer use the same release.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.1.3` tag.
