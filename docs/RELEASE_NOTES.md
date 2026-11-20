# DigitDNS v5.9.6 — lighter stats polling and locked-in behavior

This patch shrinks the dashboard's once-a-second stats requests and adds golden
locks around behavior that must not drift. No importer, filter or upstream-policy
semantics changed, and no defaults were switched.

No database schema migration is required. Existing settings, clients,
blocklists, certificates, relay agents, egress options and stored query history
are preserved. Upstream selection defaults are unchanged: Racing still maps to
Hedged.

## A lighter heartbeat

- **New `compact=2` stats mode.** Ordinary one-second polls now ask for a
  whitelist of heartbeat fields, served from its own cache key. `compact=1` and
  the full snapshot are untouched for older clients.
- **Full snapshots where they matter.** The dashboard still fetches the complete
  snapshot on boot, every fifth poll, after a failed or reconnected request, and
  when the tab becomes visible again.
- **Upstream rows keep their detail.** Heartbeats carry the compact upstream
  view; the client merges rows by address, so fields omitted by the heartbeat
  retain their last full value until the next refresh.
- In the local live check, a heartbeat was 1567 chars versus 2338 for full —
  about 33% less data on the hot path.

## Locks on working behavior

- Normalized golden hashes for AdGuard and Pi-hole importer output.
- Wire-contract tests for the compact/full stats payload and upstream fields.
- Browser-side tests for the real poll-cadence/merge code and the saved filter
  payload, extracted from the shipped `web/js/app.js`.

## Android

The companion carries the matching `android-v5.9.6` version line. The workflow's
debug APKs are attached to this release alongside the standard signed APKs for
arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.6/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.6
docker pull ghcr.io/digitrob/digitdns-agent:5.9.6
docker pull robmeister/digitdns:5.9.6
docker pull robmeister/digitdns-agent:5.9.6
```
