# DigitDNS v5.8.10 —  — sinkhole groups, faster dashboard, pasted-list repair

A minor release on top of v5.8.9. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **Custom Blocklist (Sinkhole) is now one entry in the Domains list.** The
  blocked set collapses into a single expandable row with a live domain count;
  the chevron reveals every blocked name inline, each with its normal checkbox,
  service, relay, Remove and status pill. Expanding a large sinkhole grows 50
  members at a time. The same grouping ships in the Android app.
- **Android live-log copy.** Long-press a domain in the Live Log to copy it, with
  long-press haptics and a confirmation toast. TalkBack gets the long-click
  action too.

## Changed

- **Faster dashboard loading.** The dashboard document is minified at build time
  (287 KB → 195 KB; 58 KB → 37 KB gzipped), JavaScript and CSS are served as
  Brotli sidecars (app.js 108 KB → 87 KB), and `app.js` is preloaded. The
  ten-second delay before the first stats poll is gone, so the dashboard fills
  immediately; the stats settings card is fetched on first Settings visit instead
  of on every boot.
- **Startup benchmark.** The first upstream benchmark waits ten seconds after the
  listeners are up, so its figures describe the steady state rather than boot
  contention. Adaptive mode performs one forced selection at that point and then
  sleeps until the configured interval, instead of probing twice at startup.
- **Blocked status naming.** The blocked status reads **“Blocked”** everywhere
  (web and Android), matching the filter chip and the Android app. Other status
  labels are unchanged.
- **Domains table.** The domain column is now **Domain/Blocklist** and the web
  sidebar page is **Domains/Blocklists**. The blocked pill sits in the STATUS
  column; the sinkhole title stays in the Domain column.

## Fixed

- **Pasted blocklists were stored as one unmatched pattern.** Pasting a list into
  the single-line **Block** or **Add** box used to save the whole blob as one
  space-separated entry: the Domains page showed one row and the matcher blocked
  none of the names. The Add boxes now split on whitespace, commas and
  semicolons, and the server splits any existing blob on load and save. Existing
  installs are repaired automatically.
- **Android sinkhole expansion.** Expanded member domains now render directly
  under the group instead of after the flat list.
- **Domain/Blocklist header** no longer overlaps SERVICE at narrow widths.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.9` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.9/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.9
docker pull ghcr.io/digitrob/digitdns-agent:5.8.9
docker pull robmeister/digitdns:5.8.9
docker pull robmeister/digitdns-agent:5.8.9
```
