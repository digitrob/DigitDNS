# DigitDNS v5.11.0 — AdGuard import actions and upstream picker

This release finishes the AdGuard Upstream List Import workflow on the dashboard and the Android companion. It needs no database schema migration: saved sources, checksums, provenance and rules stay in the existing `Policy` store. Existing rules, clients, certificates, relay agents, egress options, stored history and service icons are preserved.

## Dashboard: real DigitDNS actions

The importer still lives below the Domains panel and keeps its own review palette, but the review table now writes the action it shows.

- **Route status menu.** The old `Adds to custom_proxied` column is replaced by a Domains-style pill/menu with `PROXY`, `DIRECT` and `Blocked`. Applying writes the selected domain to `custom_proxied`, `custom_direct` or `custom_blocked` and removes it from the other two route lists.
- **Per-row adblock target.** Selected domains can also be appended to DigitDNS's existing filtering custom rules. This does not create a separate filter list or a new adblock source, and handwritten custom rules are preserved.
- **Hidden by default.** Domains already present in DigitDNS are hidden behind a countable notice with a `Show them` action. Search, status/coverage filters, selected-only and reset still work over the full loaded preview.
- **Saved sources.** Saved imports are first-class cards with an AdGuard favicon, domain count, date and checksum. Saving auto-names the snapshot `AdGuard Home — YYYY-MM-DD HH:mm` and deduplicates identical content by checksum. No source URL is shown or stored.
- **Favicon upstream picker.** `Managed upstream for exported rules` is now a dropdown with `This server` as the default, current upstreams, provider presets with favicons, search, and inline `Add custom upstream…`.
- **Clearer staging.** `Preview list` and `Save List` sit directly under the imported list box. The sticky footer shows route changes, adblock additions, removals and the managed upstream, plus a short `What will not change` note. `Apply to DigitDNS` stays disabled until the computed change set is non-empty and the checksum is current.
- **No generated block section.** The `Generated AdGuard block` panel is gone from both clients. The server may still return its legacy field for API compatibility, but the UI no longer presents it as an action.

## Friendly notices and removals

Raw warning strings are no longer rendered. The clients consume grouped notices for already-covered domains, broad domains, conflicts, removals and unchanged data.

The old acknowledgement wording is gone. Removals are described in plain language:

- `Off — nothing is removed.`
- `This review only — only domains shown here can be removed.`
- `Mirror source — domains this source stops naming can be removed.`

Broad and conflict awareness is preserved through inline review actions inside the relevant notice, not legalistic checkboxes. Checksum-aware apply remains in force: changing the preview, row actions, adblock targets, removal mode or managed upstream invalidates the staged apply until the list is previewed again.

## Android: full parity

The dedicated **AdGuard Upstream List Import** page under Settings → DNS policy now mirrors the dashboard flow end to end. It keeps its own semantic palette and is still not a bottom tab.

- Domains-style status menus for Proxy, Direct and Block.
- Per-row adblock targets and bulk add-selected-to-adblock.
- Hidden already-present domains with a countable show action.
- Search, filters, selected-only, reset, staged summary and disabled Apply.
- Saved-source cards with `AdGuard Home` auto-naming and checksum dedupe.
- The same favicon upstream picker with provider presets and inline add-new.
- Grouped notices and the same plain-language removals copy.
- No generated block screen or save step.

The companion carries the matching `android-v5.11.0` version line, version code 582.

## API compatibility

The v3 sync endpoints keep their paths and existing fields. Apply accepts additive optional fields for per-domain route actions, adblock domains and managed upstream selection; old clients continue to default to proxy-only imports with no adblock changes and `this-server`. Preview adds structured per-row metadata and notices while retaining the legacy `warnings` array.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.0
docker pull ghcr.io/digitrob/digitdns-agent:5.11.0
docker pull robmeister/digitdns:5.11.0
docker pull robmeister/digitdns-agent:5.11.0
```
