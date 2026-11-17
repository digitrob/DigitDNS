# DigitDNS v5.9.2 — a new domain lands at the top of the list

A Domains-list fix for the web panel, with the matching Android build. No
database schema migration is required; existing settings, clients, blocklists,
certificates, relay agents, egress options and stored query history are
preserved.

## What was wrong

The Domains table was already sorted by “Added (newest)” by default. The add
handlers wrote the rule but left its Added timestamp to the server: the server
stamped it on save and answered `{"success": true}`, so the open page kept
rendering the new row with no stamp and sorted it to the bottom. Only a reload
fetched the timestamp and moved the row to the top.

## What changed

- **The web panel stamps a new rule when it writes it**, so **Add proxied**,
  **Add blocked**, bulk import and the live-log Proxy / Block / Direct quick
  actions all appear at the top immediately.
- **Moves and scope changes keep their place.** A rename inherits the stamp left
  on the rule’s other pattern instead of looking newly added.
- **Removal drops the stamp**, so re-adding a name later gets a fresh position.
- **Millisecond timestamps** keep two adds in the same second in the right
  order; the server already accepts RFC3339 fractional seconds.

Android already stamped new routes locally; this release brings the web panel
to the same behaviour and carries the matching app version (`android-v5.9.2`).

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.2/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.2
docker pull ghcr.io/digitrob/digitdns-agent:5.9.2
docker pull robmeister/digitdns:5.9.2
docker pull robmeister/digitdns-agent:5.9.2
```
