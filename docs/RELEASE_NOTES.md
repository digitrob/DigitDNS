# DigitDNS v5.8.0 — query search and UI consistency

A minor release on top of v5.7.0. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **Query search.** An opt-in persistent query history, off by default and
  stored in a separate `querylog.db` beside the main database. Search by
  domain, client or account, action and protocol; retention defaults to 7 days
  and results are anonymised by default. Available as a Tools card on the web
  panel and a Query Log screen on Android, with CSV and JSON export.
- **Query log API.** `GET /api/v3/query-log`, settings and export endpoints,
  documented in the v3 OpenAPI spec and mirrored on the web API.
- **Multi-select filters.** Actions and protocols can be combined in both
  clients. Protocol entries carry the app's own colour in the menu and in the
  trigger.
- **One protocol palette.** UDP cyan, TCP blue, DoT emerald, DoH purple, DoQ
  amber, DoH3 orange and DNSCrypt fuchsia, shared by the web panel, Android and
  the live query log, with a light-mode step.
- **Copy feedback.** Copy actions confirm in place — a check glyph on the web,
  a tinted control in the portal, and a green check for 1.2 s on Android.

## Changed

- **Shared fields and buttons.** Login, change-password, diagnostics, add/edit
  client, bulk import, the API allow-list and the Domains/live-log toolbars use
  the shared `.panel-field` and `.panel-btn` styles. `.panel-field-sm` covers
  dense toolbars and `.panel-field-end-icon` keeps an in-field glyph clear.
- **One primary action per form.** Modal and full-width form commits use the
  primary button style; the last hand-written gradient buttons are gone.
- **Settings and phone navigation.** Settings is grouped under six captions
  above the sticky section jump bar. The phone bottom bar drops to six targets
  (Dashboard, Domains, Filters, Users, Live Log, More), with the rest in a More
  sheet that reuses the same tab wiring.
- **Compact query filters.** Both clients show the first selected chip plus a
  `+N` count, so more selections never grow the card or stretch the domain and
  client fields.
- **Android.** Query Log scrolls in landscape or any viewport shorter than the
  content; portrait still fits without page scrolling. Dialog corners now match
  card corners.

## Fixed

- Query-card dropdowns were clipped by paint containment; the card releases it
  and the menus flip up or down based on viewport space and scroll when long.
- A wide first filter label clipped the `+N` counter; the chip now yields width
  so the number always shows.
- Toggled buttons could stay visible because `.hidden` lost to a component
  display rule; the two-class guard now matches `.badge.hidden`.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.0` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.0
docker pull ghcr.io/digitrob/digitdns-agent:5.8.0
docker pull robmeister/digitdns:5.8.0
docker pull robmeister/digitdns-agent:5.8.0
```
