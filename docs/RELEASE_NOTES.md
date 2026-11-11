# DigitDNS v5.5.6 — biometric login and a tighter dashboard

A feature and performance release on top of v5.5.5. No resolver, API or database
schema migration is required. Existing settings, clients, blocklists and
certificates are preserved.

## Android

- **Biometric login.** A new switch in **Settings → App**, directly below the
  theme choice, requires a fingerprint, face or device PIN to open the app.
  - Enabling it shows the prompt first; the preference is saved only after a
    successful authentication.
  - Locks on cold start and after a **60-second** background grace period.
  - Device PIN/pattern is accepted as a fallback.
  - The lock screen offers **Unlock** and **Sign out**.
- App version **5.5.6 (547)**.

## Panel

- **Uniform 12 px dashboard spacing.** Metrics, main and history grids and the
  section stack now share one gap. The custom layout editor uses the same 12 px
  column and row gap, so spacing no longer changes with the viewport or a saved
  layout.
- **Compact stats polling.** The 1 Hz dashboard poll requests only the live
  fields; the full document is fetched every fifth poll and the slow CPU, RAM,
  system-memory and agent counters are merged from it.
- **In-place upstream rows.** The resolver card updates latency, health dot and
  tooltip without clearing and rebuilding the list every second.

## Backend

- **Blocklist match path.** The matcher passes an already-normalized name to the
  blocklist index, which no longer lowercases and trims it a second time.
- **Agent list cache.** The panel's agent list is cached for two seconds and
  invalidated on create/update/delete/enable/disable.
- **Metrics cache.** `/api/metrics` reuses the Prometheus text for one second.

No DTO, auth, route or schema changes.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.6
docker pull ghcr.io/digitrob/digitdns-agent:5.5.6
docker pull robmeister/digitdns:5.5.6
docker pull robmeister/digitdns-agent:5.5.6
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.6/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.6` tag.
