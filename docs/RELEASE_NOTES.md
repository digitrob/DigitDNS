# DigitDNS v5.9.3 — live-log decisions win, and the panel stays responsive

This release makes a manual live-log decision authoritative, adds the additive
configuration importer to the web panel and Android app, and removes several
sources of intermittent DNS and UI stalls found in the resolver, database and
dashboard hot paths.

No database schema migration is required. Existing settings, clients,
blocklists, certificates, relay agents, egress options and stored query history
are preserved.

## Live-log decisions are now authoritative

- **Direct / Proxy / Blocked chosen from the live log overrides every other
  rule.** The resolver checks the live-log tier before custom A/CNAME records,
  custom groups and lists, blocklists, forced-direct rules and ordinary proxy
  rules.
- **A wildcard decision is an exception for its whole subtree.** A manual
  `*.example.com` decision applies even when a lower tier has an exact rule for
  `tracker.example.com`; the most specific manual rule wins when two live-log
  decisions overlap.
- **The save is awaited and the page repaints from the server's normalised
  state.** A failed save reloads the real configuration instead of leaving an
  optimistic row that disappears later. A scope click followed by an action no
  longer issues two writes for one decision.
- **Scope previews and toasts now name exactly what is written** and warn only
  when the clicked host itself is left out.

## Configuration importer

- **Analyse first, then apply.** The importer shows what will be added and
  writes nothing until the operator confirms.
- **Additive only.** Existing records, clients, rules, lists, rewrites and
  filtering settings are never overwritten, and secrets are never imported.
- **Large lists become uploaded FilterLists.** Regex rules beyond the resolver
  caps are reported and downloadable instead of being silently dropped.
- **Pi-hole v6 is parsed locally in the browser** with SQLite-WASM. Raw
  archives and query history stay on the operator's device.
- **Redacted configuration export and one-click undo** are included. Android
  has the matching importer flow.

## Performance and reliability

- Filter refreshes cache each list's compiled index and merge it, instead of
  analysing a body and then reparsing every enabled list. A batch of due lists
  rebuilds once, and a `304 Not Modified` refresh reuses the cached parse.
- Query logging no longer performs bbolt writes on the DNS path: the
  anonymisation salt is loaded once at startup.
- Rule saves use a single bbolt transaction instead of eleven, and client-index
  reloads build off-lock before swapping in the new index.
- The Domains table fills its viewport in one render, icon replacement is scoped
  to the container just painted, and stale animation-frame batches cannot
  duplicate live-log rows after a tab switch.
- Relay target lookups are cached for a short window and bounded separately from
  the relay handshake, so a slow system resolver cannot consume the whole
  handshake budget.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.3/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.3
docker pull ghcr.io/digitrob/digitdns-agent:5.9.3
docker pull robmeister/digitdns:5.9.3
docker pull robmeister/digitdns-agent:5.9.3
```

The Android companion carries the matching `android-v5.9.3` version line.
