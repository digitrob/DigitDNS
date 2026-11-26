# DigitDNS v5.12.1 — Domains page: menus that scroll, bulk actions that stay scoped

A focused patch for the Domains page. The per-domain **Service** and **Relay** menus can now be scrolled, bulk actions act only on the rows you ticked, and paused domains sort below the active ones. No database schema migration is required.

## Menus that scroll

The Service and Relay pickers are fixed popups, positioned over the row that opened them. They were closed by a capture-phase `scroll` listener on `window` — which also saw the menu's own scrollbar. With a long service list, the first wheel or trackpad movement closed the menu before you could reach a lower entry.

The listener now ignores scroll events that originate inside the open menu. Page and table scrolls still dismiss it, as do an outside click, `Esc` and a window resize.

## Bulk actions stay scoped

Delete, Pause/Resume, Assign service and Assign relay/egress all iterate the exact selection set and refuse to run when nothing is ticked. The delete dialog now names the count and previews the first ten selected domains before confirming, so an accidental **Select all** is visible before anything is removed. Removing a domain also clears its service and relay assignment.

## Active first, paused last

The default **Added (newest)** order keeps non-paused domains on top and sorts paused domains below, each group still newest-added first. **Added (oldest)** follows the same paused-last rule; **Name A–Z** and **Z–A** stay purely alphabetical, and **Status (on first)** is unchanged.

## Search across services

Typing in the Domains search box no longer intersects the active service filter. The search runs across the whole managed list, and the result count and empty state describe the search rather than the selected service.

## Verification

Dashboard asset tests pass after the minified rebuild (`app.min.js`, `index.min.html` and the Brotli sidecars). No database schema migration is required.

The Android companion moves to **android-v5.12.1**, version code **591**. No Android code changed in this release.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.12.1/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.12.1
docker pull ghcr.io/digitrob/digitdns-agent:5.12.1
docker pull robmeister/digitdns:5.12.1
docker pull robmeister/digitdns-agent:5.12.1
```
