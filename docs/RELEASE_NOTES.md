# DigitDNS v5.8.13 — live-log DNS response detail

A feature release on top of v5.8.11. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **See what a query actually resolved to, from the live log.** Both the web
  dashboard and the Android app can open a DNS-response panel for any row. It
  shows the answer section in the order the resolver returned it — the CNAME
  chain followed by the address records, each with its own TTL:

  ```
  Response code  NOERROR
  Response       CNAME: proud-forest-0346aa81e.4.azurestaticapps.net. (ttl=30)
                 CNAME: azurestaticappstm4.trafficmanager.net. (ttl=30)
                 A: 51.105.228.104 (ttl=30)
  ```

  It also carries the processing latency, the client and upstream transports,
  the cache state, the query type, the block source and the relay.
- **Web popover.** Hovering the (i) button in the ACTION column opens a panel
  anchored beside it, with a caret pointing at the button. The pointer can move
  into the panel; moving away closes it. Values keep the same protocol colours
  used everywhere else in the panel.
- **Android dialog.** Tap the domain on a live-log row to open a compact
  DNS-response dialog; long-press still copies the domain. The per-row info
  button was removed, so the row keeps its width for the domain.

## Changed

- **Sub-millisecond latency is shown, not rounded.** Cache hits, blocks and
  proxied names are usually answered in well under a millisecond and used to read
  `0 ms`; they now read values such as `0.042 ms`.
- **Answer records are grouped under one Response field**, one record per line,
  instead of repeating the label for each address.
- Labels: **Latency**, **Response code**, **Response**, **Query Type**; block
  categories are spelled **Ads**, **Tracker**, **Malware**.

## Fixed

- **Blocked queries no longer show the null-IP sinkhole (`0.0.0.0`) as a
  resolved answer.** A block is a policy reply, not a resolution.
- **DNSCrypt keeps readable contrast on the light theme** in the response panel.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.13` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.13/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.13
docker pull ghcr.io/digitrob/digitdns-agent:5.8.13
docker pull robmeister/digitdns:5.8.13
docker pull robmeister/digitdns-agent:5.8.13
```
