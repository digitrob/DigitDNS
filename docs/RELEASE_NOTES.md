# DigitDNS v5.10.2 — AdGuard Upstream List Import polish

This patch refines the AdGuard coverage sync introduced in v5.10.1 on both clients. It needs no database schema migration: saved sources, checksums, provenance and rules stay in the existing `Policy` store. Existing rules, clients, certificates, relay agents, egress options, stored history and service icons are preserved. Upstream selection defaults are unchanged: Racing still maps to Hedged.

## Dashboard: AdGuard Upstream List Import

The full-width card on the Domains tab is now called **AdGuard Upstream List Import** and follows the Domains page's visual language.

- **A clear three-step explainer.** Paste the `[domain1/domain2/]upstream` list, review the comparison, then apply deliberately. The page states that DigitDNS is the primary rule store and that handwritten AdGuard sections are never rewritten automatically.
- **A Domains-style comparison table.** The header stays in place while only the domain rows scroll. Existing panel fonts, buttons, checkboxes, dividers and empty states are reused.
- **Bulk selection.** Select all domains that need to be added to DigitDNS, clear imports, select all domains to send back to AdGuard, clear exports, filter by domain/group/upstream, and show only selected rows. Apply stays disabled until an import, export or removal is staged.
- **A palette of its own.** `Already in DigitDNS`, `Add to DigitDNS`, `Conflict`, `Send back to AdGuard`, `Broad` and `External upstream` have dedicated tones. They are deliberately different from live-log actions, history states, DNS transports and service badges, and a legend makes the meaning explicit.
- **No source URL.** Saved sources continue to work from their pasted text. The URL field and URL-refresh button are gone from the dashboard; server endpoints remain compatible for existing clients.

## Friendly notices instead of raw warning spam

The preview API still returns the original `warnings` array for compatibility, but the dashboard and Android app now render grouped `notices`:

- One compact notice names any upstream targets that are not this DigitDNS server and explains that their domains can be imported while AdGuard keeps routing them there until its upstream settings change.
- One compact notice reports the number of high-traffic domains with a few examples, and broad rows carry their own pill.
- Conflicts get one short explanation rather than a per-domain warning wall.

The additive preview fields are `notices`, `broad_count`, `foreign_targets` and a per-row `broad` flag. The risk acknowledgement and exact-preview checksum rules are unchanged.

## Android: dedicated settings page

The companion gains a separate **AdGuard Upstream List Import** page under Settings → DNS policy. It is not a new bottom tab. It mirrors the dashboard flow: saved-source selector, display name, enabled switch, managed upstream, pasted list, preview, bulk selection, bounded apply, and copy/share of the generated managed block. It uses Material 3 panel components and its own semantic palette so its review states cannot be confused with existing action or status pills.

The companion carries the matching `android-v5.10.2` version line, version code 581.

## Fixed

The repository is `gofmt` clean again, including the AdGuard sync files added in v5.10.1.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.10.2/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.10.2
docker pull ghcr.io/digitrob/digitdns-agent:5.10.2
docker pull robmeister/digitdns:5.10.2
docker pull robmeister/digitdns-agent:5.10.2
```
