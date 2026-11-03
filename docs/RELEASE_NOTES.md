# DigitDNS v4.0.3

A dashboard-focused patch. No DNS resolution, relay, API, database or
configuration behaviour changed.

## What changed

- **The dashboard brand mark is now the mobile app icon**, cropped to remove the
  outer dark-blue padding and enlarged to 48px on desktop and 36px on mobile.
- **The Live Log has a refresh button**, placed to the left of the search field.
  It reconnects the stream, reloads the recent-query snapshot, keeps the active
  search/filter/sort, and resumes a paused log. The control spins while
  connecting and cannot stack reconnects.
- **Settings is now a compact card grid.** Two columns at 1100px and wider, one
  column below; wide forms keep a full row while simple cards share one.
  Padding and gaps are reduced without hiding fields or status text.

## Upgrade

Pull the new image and recreate the hub and agent containers. Keep the existing
`/app/data` and `/app/certs` volumes (or `/data` for the agent) — no migration is
required.

```bash
docker pull ghcr.io/digitrob/digitdns:4.0.3
docker pull ghcr.io/digitrob/digitdns-agent:4.0.3
```

The Android app remains 4.0.0; this release does not change it.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `checksums.txt`, `checksums.sig`

Full list of changes: [docs/CHANGELOG.md](CHANGELOG.md).
