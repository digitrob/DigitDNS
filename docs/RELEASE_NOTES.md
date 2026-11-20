# DigitDNS v5.9.7 — focused live log and reclaimable storage

This patch makes the live log cheaper to keep open and easier to read, adds a
safe way to reclaim bbolt high-water space, and removes a false "leave page?"
warning after signing in or out. No database schema migration is required.
Existing settings, clients, blocklists, certificates, relay agents, egress
options and stored query history are preserved. Upstream selection defaults are
unchanged: Racing still maps to Hedged.

## A live log that explains itself

- **Follow control.** Scrolling away from the newest rows stops DOM inserts;
  a *Latest* chip counts the arrivals buffered while you read, and one click
  returns to the live head. Clearing, reconnecting and resuming all reset the
  count so it never describes rows you cannot see.
- **Dense rows and click-to-filter.** A local, per-browser density toggle fits
  more rows without hiding columns. Clicking a domain or client cell writes that
  exact value into the search box; clicking it again clears the filter.
- **Latency tiers.** Four colour bands make slow answers visible without
  turning every row into a different colour.
- **No idle background streams.** A hidden tab suspends the SSE connection and
  its polling fallback, then reopens through a fresh ticket. The server's
  recent-query snapshot covers the time away.
- **Dropped frames are reported.** Each subscriber has a bounded queue and a
  drop counter. Overflow emits one `dropped` event with the total, shown in the
  status tooltip so a gap has an explanation. A brief drop does not change the
  Live label.

Ranked history cards now show ten entries by default, with a 10 → 50 → 100
toolbar control. The API cap of 100 is unchanged.

## Storage maintenance

Two new pre-daemon commands make bbolt high-water growth visible and fixable:

```bash
sudo systemctl stop digitdns
sudo digitdns db-stats       # size, free bytes/pages and bucket key counts
sudo digitdns compact-db     # rewrite in place, then report before/after
sudo systemctl start digitdns
```

Both commands need the daemon stopped because bbolt takes an exclusive file
lock. Compaction runs through the same verified rewrite path used by the
encryption migration, and re-inspects the file afterwards. Take a backup before
running it on production data. `digitdns help` documents the maintenance window.

## Android

The companion carries the matching `android-v5.9.7` version line (version code
577). The workflow's debug APKs are attached to this release alongside the
standard signed APKs for arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.7/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.7
docker pull ghcr.io/digitrob/digitdns-agent:5.9.7
docker pull robmeister/digitdns:5.9.7
docker pull robmeister/digitdns-agent:5.9.7
```
