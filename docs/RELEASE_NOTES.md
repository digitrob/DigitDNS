# DigitDNS v5.8.14 — query history parity and a live log that tells the truth

A polish and correctness release on top of v5.8.13. No database schema migration
is required; existing settings, clients, blocklists, certificates, relay agents,
egress options and stored query history are preserved.

## Added

- **Query history is paged, not truncated.** `/api/query-log` now returns a
  cursor (`next_cursor` / `has_more`) built from the stored record key, so both
  the web dashboard and the Android app can page through the whole retention
  window instead of seeing the newest slice and nothing else. One page is 100
  records by default, 500 at most.
- **Full-answer history on Android.** History rows open the same DNS-response
  dialog as the live log: answer chain with TTLs, response code, latency,
  transports, cache state, upstream, block source and relay. Both screens now
  share one `DnsDetails` model, so the two views cannot drift apart.
- **Filters that match the data.** Query history gained time range, query type,
  response code and cache-state filters on both clients, alongside the existing
  domain, client, action and protocol filters. Query type and response code are
  accepted as either wire numbers (`28`) or names (`AAAA`).
- **Visible connectivity.** The live log has a status pill that says `Live`,
  `Connecting`, `Reconnecting`, `Polling` or `Offline`. The Android live log
  shows the stream state it was already tracking but never displayed. The panel
  reacts to the browser going offline and reconnects immediately when the network
  returns instead of waiting out the 30-second backoff.
- **Storage is visible and bounded.** The query-log card shows how many records
  are stored and how much space they occupy against the configured cap, and the
  cap is now editable (64 MB to 2 GB). Cleanup enforces a storage cap.

## Changed

- **CSV and JSON exports stream the whole result set** — up to 100 000 records —
  instead of the first 1000 rows, and report truncation in an
  `X-Query-Log-Truncated` trailer when the ceiling is reached. CSV gained
  `qtype_name`, `rcode_name`, `upstream`, `resolved_ips` and `answers`.
- **The fallback poll revalidates.** `/api/stream/recent` serves an ETag, so the
  two-second fallback costs a 304 with no body when the ring has not changed.
- Query-log cleanup runs every 15 minutes and deletes in bounded batches, so a
  large retention sweep no longer stalls the store.
- The mobile DNS-response panel becomes a full-width bottom sheet; the desktop
  popover keeps its caret and stays anchored beside the (i) button.

## Fixed

- **Saving query-log settings no longer resets fields the form does not edit.**
  The web card used to send `opt_out: []` and `max_mb: 256` on every save,
  silently un-excluding every opted-out client and resetting the storage cap.
- **Time filtering honours both bounds.** The descending walk now breaks at the
  lower bound and skips rows newer than the upper bound; an upper bound used to
  return nothing.
- **Clearing history survives a pending flush.** `Clear` is routed through the
  writer goroutine, so a batch already queued cannot land after the delete.
- **Records are serialised before the write transaction opens**, shortening the
  time the query store is write-locked.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.14` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.14/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.14
docker pull ghcr.io/digitrob/digitdns-agent:5.8.14
docker pull robmeister/digitdns:5.8.14
docker pull robmeister/digitdns-agent:5.8.14
```
