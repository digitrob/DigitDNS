# DigitDNS v5.9.0 — settings, split where you look for them

A settings-focused release for the web panel and the Android app. No database
schema migration is required; existing settings, clients, blocklists,
certificates, relay agents, egress options and stored query history are
preserved.

## A Resolvers tab

Settings had grown to eighteen cards in one scroll, mixing server access,
subscriber TLS, per-domain policy, resolver plumbing and history. The five cards
that configure how the resolver speaks to clients now live on their own page:

- **Client Transports** — plain DNS, DoT, DoH, DoH3, DoQ, DNSCrypt, mTLS
- **DNSSEC Validation**
- **Proxy Leak Protection**
- **Proxy IPv6**
- **DoH Access Tokens**

Resolvers has its own sidebar entry, phone More-sheet entry, clean URL
(`/resolvers`), boot route and jump bar. Settings keeps the other thirteen cards:
server location, 2FA, server update, hidden admin path, administrator
credentials, subscription portal, SSL/TLS, blocked domains, DNS rewrites, policy
groups, traffic history, alerts and about.

Nothing about the moved cards changed: same ids, same endpoints, same save
behaviour. The tab-open refresh follows them, so DNSSEC and proxy-leak status are
read when Resolvers opens.

## Settings that explain themselves

- **Every section has a jump chip.** The sticky bar also tracks the section at
  the top of the viewport, so the highlighted chip matches what you are looking
  at rather than the last one clicked.
- **The command palette searches every section.** It used to index eight
  hand-picked chips; it now derives its Settings and Resolvers entries from the
  jump bars, so “DNSSEC”, “DoH tokens” and “About” are all reachable from ⌘K.
- **Every mutating card has a save-model badge.** The four rule cards that write
  as you edit now say “Auto-saved” instead of staying silent.
- **“Save to apply” cards show an “Unsaved changes” note** once a field is
  edited, and clear it when that card’s save succeeds. The note never blocks
  navigation; a reload with a pending edit asks the browser to confirm.

## Settings open in their saved state

- **The Traffic History card no longer flips its checkboxes.** The card is read
  when Settings opens; its controls now stay hidden behind a short “Loading
  history settings…” line until the saved policy arrives, so they appear already
  checked or unchecked instead of moving after the read.
- **Android does the same on every screen with a server-backed switch.**
  Traffic History shows a loading/retry card until the policy lands, SSL/TLS
  waits for the settings snapshot before drawing the renewal switches, and the
  Query Log and Egress Options toggles wait for their first load.

## Android: a settings hub that fits a phone

The Settings tab was a single horizontally scrolling row of 23 chips. It is now
a grouped list with a search field:

- **Resolvers** — Upstreams, Client Transports, DNSSEC & Proxy Leak, Proxy IPv6,
  Access (DoH tokens and allowlists)
- **Server & access** — Servers, Network & Server, SSL/TLS, Subscription,
  Security, Egress Options
- **DNS policy** — Ad Blocking, Groups, Policies
- **Monitoring** — Traffic History, Query Log, Alerts, System Log, Diagnostics
- **App** — App, REST API, Connect Guide, About

Selecting a row opens the existing screen with a back arrow; the system back
gesture returns to the list. Search matches section names and English keywords
(“wireguard” finds Egress Options, “certificate” finds SSL/TLS). The five bottom
tabs are unchanged.

## Fixed

- The transports notice used to call a tab id that did not exist, hiding every
  panel and leaving a blank page. It now opens Resolvers and scrolls to Client
  Transports.
- Client Transports is no longer filed under “Subscriber portal & TLS”.
- The phone More button stays lit while Resolvers is open.
- **Android Domains “Added (newest)” now means newest across every status.**
  With no chip selected the default view concatenates proxied, paused, blocked
  and direct rows; reversing that list only reversed the status groups, so the
  newest proxy sat at the bottom. The list now sorts by the server's stored
  added time, and new, moved and removed rules keep that stamp correct.

## Upgrade

In-place upgrades from the dashboard are supported. Existing bookmarks to
`/settings` keep working; `/resolvers` is a new clean URL. The Android app is a
side-loaded companion and keeps its own version line (`android-v5.9.0`).

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.0
docker pull ghcr.io/digitrob/digitdns-agent:5.9.0
docker pull robmeister/digitdns:5.9.0
docker pull robmeister/digitdns-agent:5.9.0
```
