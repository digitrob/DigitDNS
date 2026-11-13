# DigitDNS v5.8.2 — AdGuard custom rules, command palette, performance

A minor release on top of v5.8.1. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents and egress
options are preserved.

## Added

- **AdGuard-syntax custom filtering rules.** A text area on the Ad Blocking card
  (web + Android) accepts AdGuard/uBlock network rules (`||domain^`, `@@||`,
  `$dnsrewrite=`, `$important`, hosts format). Rules compile into the same index
  as your blocklists.
- **`$dnsrewrite` support.** `$dnsrewrite=IP` (A/AAAA), `$dnsrewrite=host`
  (CNAME), `$dnsrewrite=TXT "text"`, and `$dnsrewrite=NXDOMAIN/REFUSED/SERVFAIL`.
- **Command palette.** Ctrl/Cmd+K opens a fuzzy search over tabs, settings
  sections and global actions (theme toggle, sign out) on the web panel.
- **Android status widget.** A 2×1 home-screen widget showing queries today and
  the live query rate; tap to open the app.
- **Android toggle widget.** A 1×1 widget showing filtering on/off; tap opens
  the app's Ad Blocking page.
- **Android switches contrast.** Switches in the grey theme now use the success
  colour for the checked track (was nearly white).

## Changed

- **Resolver hot path.** System metrics sampled once per second in
  `telemetryLoop` instead of on every `/api/stats` call. `normalizeProto` uses
  `EqualFold` (zero alloc per query). Aggregator uses a per-bucket mutex and an
  atomic current-bucket pointer.
- **Web theming.** Non-theme-mapped `sky-*` and `violet-*` Tailwind utilities
  replaced with theme-mapped `cyan-*`/`purple-*` across DoT settings, custom
  policy groups, the top-upstreams icon, the 2FA card and the zap icon.
- **Adaptive selection.** `adaptiveHealthy` now accepts resolvers that have a
  successful probe, so the Force button switches to the fastest probed resolver
  even if it never served live traffic. The timer resets after each forced run.

## Fixed

- **Upstream Force button.** Not all the time did it switch to the fastest
  resolver (probed-but-unmeasured resolvers were excluded). Fixed.
- **Android detekt.** `TransportChips` no longer exceeds the
  cyclomatic-complexity threshold (the transport colour/label lookup moved to
  the theme).
- **Android alert channel name collapse.** The `weight(1f)` field now has a
  real row to fill instead of being squeezed beside the kind selector, switch
  and delete button.
- **Web alert type picker.** The native `<select>` (which cannot render icons)
  was replaced with a pictogram menu.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.2` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.2/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.2
docker pull ghcr.io/digitrob/digitdns-agent:5.8.2
docker pull robmeister/digitdns:5.8.2
docker pull robmeister/digitdns-agent:5.8.2
```
