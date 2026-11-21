# DigitDNS v5.10.1 — AdGuard coverage sync and unified palettes

This patch adds a safe AdGuard ↔ DigitDNS coverage comparison to the dashboard and makes action/status pills and DNS transport colours agree across the dashboard and Android. It needs no database schema migration: sync state lives in the existing `Policy` store. Existing rules, clients, certificates, relay agents, egress options, stored history and service icons are preserved. Upstream selection defaults are unchanged: Racing still maps to Hedged.

## AdGuard coverage sync

The Domains tab now carries a full-width **AdGuard Sync** card. Paste an AdGuard upstream config, optionally name it and give a URL, then preview before anything is written.

- **Every active bracketed upstream group counts.** The parser reads each active group whatever its target, converts exact and `*.wildcard` tokens, and ignores comments, disabled rules and default lines. Regex, malformed and unsupported tokens are reported as warnings rather than silently imported.
- **Four preview buckets.** Domains DigitDNS already covers stay untouched. AdGuard-only domains are proposed for `custom_proxied` and checked by default. DigitDNS-only domains are proposed back to AdGuard as an unchecked managed block. Conflicts and broad domains are called out before apply.
- **Back to AdGuard, without rewriting the source.** Select which DigitDNS-only domains to export and copy or download a managed block delimited by `BEGIN DIGITDNS SYNC — managed` / `END DIGITDNS SYNC — managed`. Handwritten AdGuard sections are never rewritten in this release; URL write-back and two-way auto-sync are out of scope.
- **Apply is bounded by the preview.** The request carries the exact preview text and its checksum, so a changed source cannot be applied from a stale view. Import and managed selections are revalidated server-side against the preview.
- **Removals stay conservative.** The default is `keep everything`. `synced_only` removes only domains this source previously synced and no longer names. `mirror` makes the source authoritative for custom proxied domains, requires a separate checkbox and a second confirmation, and preserves domains owned by other sources.

Saved sources persist their last checksum, provenance and last-good text. URL userinfo is redacted before storage, and a failed URL refresh keeps the last good source instead of replacing it with an error page.

The backing endpoints are documented in the v3 OpenAPI spec: source list/save/delete/refresh/export, `/api/v3/sync/adguard/preview` and `/api/v3/sync/adguard/apply`.

## One visual language

- **Text-only pills.** Live-log, history and Domains action badges now use the same text-only `.badge` shape. Pictograms were removed from the pills; relay flags, service icons, alert-channel type icons, copy buttons, block-category chips, `RAM`, the live-connection dot and the changed-row marker are unchanged.
- **Canonical action tones.** Proxy outcomes are cyan. `BLOCK`, `PROXY_FAILED`, `RATELIMIT`, `QUOTA` and `SERVFAIL` are red. `DIRECT` and `CACHED` are green, `STALE` amber, `CUSTOM` purple, and unknown actions muted.
- **Canonical transports.** UDP is cyan, TCP blue, DoT green, DoH purple, DoQ amber, DoH3 orange and DNSCrypt fuchsia on both clients. Cross-platform tests keep the palettes from drifting.
- **Labels stay honest.** A cached answer still reads `DIRECT` in the live log and `CACHED` in history.

## Android

The companion carries the matching `android-v5.10.1` version line (version code 580). Action and protocol colour decisions are centralised in shared palettes, `SERVFAIL` follows the failure tone, and the existing live-log marker, service icons, navigation and stored rule data are unchanged. The workflow's debug APKs are attached to this release alongside the signed APKs for arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.10.1/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.10.1
docker pull ghcr.io/digitrob/digitdns-agent:5.10.1
docker pull robmeister/digitdns:5.10.1
docker pull robmeister/digitdns-agent:5.10.1
```
