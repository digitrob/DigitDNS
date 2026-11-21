# DigitDNS v5.10.0 — honest history and cross-client icons

This minor release makes the Live Log clearer after quick actions and brings
service-icon suggestions to the Android companion. No database schema migration
is required. Existing settings, clients, blocklists, certificates, relay agents,
egress options, stored query history and stored service icons are preserved.
Upstream selection defaults are unchanged: Racing still maps to Hedged.

## A quick action no longer rewrites the past

- **Historical badges stay historical.** Saving Proxy, Block or Direct from a
  live-log row leaves the query's original action untouched. The row gains a
  muted, struck-through badge with a tooltip naming the new rule and explaining
  that the row shows the answer at query time.
- **Session-only state.** Markers are keyed by query id (with a domain/time/client
  fallback), survive polling refreshes and pause/resume, and are dropped when the
  log is cleared or the client restarts. A failed save never marks a row.
- **Explain stays read-only.** Diagnostics and Tools Explain still resolve a
  domain without creating a query or marking live-log history.

## Service icons come from a library

- **Type a name, domain or URL.** The Domains editor reduces it to a search term,
  so `Netflix`, `netflix.com`, `https://www.netflix.com/browse` and
  `ads.netflix.co.uk` all offer the Netflix logo. Image URLs and IP addresses do
  not trigger a search.
- **Both clients agree.** The dashboard and Android use the same Iconify ids and
  store the same explicit SVG URL in `service_icons`. Monochrome sets receive a
  neutral tint; colourful logo and emoji sets keep their own palette.
- **Stale results cannot repaint the field.** A 350 ms debounce and a sequence
  guard discard replies from earlier edits or a closed dialog, and choices render
  inside the existing dialog scroll instead of adding a nested one.
- **Works with both credential modes.** The new `GET /api/v3/icons/search`
  endpoint delegates to the dashboard's cached Iconify proxy and accepts either
  a dashboard session or the master API key.

## Android

The companion carries the matching `android-v5.10.0` version line (version code
579). It adds the live-log change marker and the service-icon suggestions while
keeping the existing layouts, navigation and all stored rule data. The
workflow's debug APKs are attached to this release alongside the standard signed
APKs for arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.10.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.10.0
docker pull ghcr.io/digitrob/digitdns-agent:5.10.0
docker pull robmeister/digitdns:5.10.0
docker pull robmeister/digitdns-agent:5.10.0
```
