# ⚡ DigitDNS

[![Release](https://img.shields.io/github/v/release/digitrob/DigitDNS?display=tag&style=for-the-badge&label=Release)](https://github.com/digitrob/DigitDNS/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/digitrob/DigitDNS/ci.yml?branch=main&style=for-the-badge&label=CI)](https://github.com/digitrob/DigitDNS/actions/workflows/ci.yml)
[![Android](https://img.shields.io/github/actions/workflow/status/digitrob/DigitDNS/android.yml?branch=main&style=for-the-badge&label=Android)](https://github.com/digitrob/DigitDNS/actions/workflows/android.yml)
[![Docker](https://img.shields.io/github/actions/workflow/status/digitrob/DigitDNS/docker.yml?branch=main&style=for-the-badge&label=Docker)](https://github.com/digitrob/DigitDNS/actions/workflows/docker.yml)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge&logo=gnu)](LICENSE)

DigitDNS is a self-hosted SmartDNS resolver, a TLS pass-through relay, a DNS sink
with ad blocking and a subscriber panel in one static binary. One install covers
DNS resolution, geo-blocking bypass and ad blocking. Point a phone, console, TV or
router at it and each domain gets one of three answers: resolve normally, refuse
or answer from the sink, or connect through your server. The panel manages rules,
subscribers, blocklists, certificates and upstream resolvers; a customer portal
and a native Android app give subscribers their own view.

**Looking for a self-hosted DNS server, a Pi-hole or AdGuard Home alternative
with ad blocking, or an SNI proxy for geo-restricted services?** DigitDNS
combines all three in one install, one panel and one Android companion.

It is a fork of [HyperDNS](https://github.com/IzumiRain/HyperDNS) by HyperRAIN,
maintained by [Digitrob](https://github.com/digitrob). It is free, open source
(AGPL-3.0), and runs on your own VPS or home server with no account and no cloud
dependency. Use it only on servers and networks you are allowed to operate.

<p align="center">
  <img src="docs/screenshots/v5-web-dashboard.jpg" alt="DigitDNS web dashboard" width="100%">
</p>

<p align="center">
  <a href="#quick-start">🚀 Quick start</a> •
  <a href="#installation">📦 Install</a> •
  <a href="#features">🌟 Features</a> •
  <a href="#client-area">👤 Client area</a> •
  <a href="#android-app">📱 Android</a> •
  <a href="#operations">🔧 Operations</a> •
  <a href="#security">🛡️ Security</a> •
  <a href="#screenshots">📸 Screenshots</a> •
  <a href="#documentation">📚 Docs</a>
</p>

---

## What's new in 5.5.2

**Upstream latency you can trust, and a cleaner provider browser.**

- **New resolvers benchmark automatically** when added.
- **Benchmarks warm up and take the median of three probes**, so TLS/QUIC
  handshake spikes no longer show up as resolver latency.
- **Live p50 needs three samples** before replacing the probe median; one slow
  query can no longer make a healthy resolver look broken.
- **Web provider browsing is smoother:** checked servers toggle off cleanly,
  the field suppresses browser address autofill, dropdown type is larger, and
  the upstream card title aligns with the chart card.

## What's new in 5.5.1

**A DNS-over-QUIC fix and installer sync.**

- **DoQ now sends STREAM FIN** after each query, so strict public resolvers
  such as AdGuard and Quad9 answer probes and report latency instead of leaving
  the upstream unmeasured.
- **The offline-bundle installer opens DNSCrypt 5443**, matching the online
  installer.

## What's new in 5.5.0

**Every client-facing transport in one place, with DNSCrypt, mTLS and token
authentication — plus a steadier web and Android panel.**

- **Client Transports card** on web and Android: Plain DNS, DoT, DoH, DoQ,
  DoH3, DNSCrypt and the mTLS policy, each with a port, endpoint and switch.
  Plain DNS and DoH can be disabled independently; DoT defaults to on; DoH3
  stays on and shares the DoH port.
- **DNSCrypt server** on UDP and TCP 5443, with encrypted provider keys, a
  signed certificate, a copyable stamp and live status.
- **DoH bearer tokens**, **DoT SNI-token authentication**, and **mTLS** for
  DoT, DoH, DoH3 and DoQ from an operator-supplied client CA.
- **Transport edits autosave** on web and Android; the global Save button is
  gone. Incomplete token or mTLS forms stay local until they are valid.
- **DNSSEC saves apply live** and the cache is flushed; the button is now
  **Save & Apply DNSSEC Settings**.
- **Web transport switches no longer flicker**, and Android settings pages load
  with card skeletons instead of a full-screen spinner. Android transport
  switches now save reliably and keep a fixed right-aligned slot.
- **RAM cards lead with the instance's own allocation**; machine-wide server
  RAM and usage are secondary on web and Android.
- **Public DNS provider presets:** the upstream add box searches a curated
  top-ten list, grouped by provider with bundled logos and every supported
  transport; a custom address can still be typed directly.

## What's new in 5.4.3

**A faster dashboard and clearer health/blocking state.**

- **API gzip** cuts `/api/filtering` from ~22 KB to ~5.3 KB, `/api/metrics` from
  ~7.7 KB to ~1.7 KB, and `/api/config` from ~2.2 KB to ~0.9 KB; tiny responses
  stay uncompressed.
- **Minified assets** are served under the same URLs: `app.js` ~153 KB → ~80 KB
  gzipped and `style.css` ~39 KB → ~11.8 KB. A source-hash test keeps the
  minified output in sync.
- **Chart.js is deferred** until the first stats poll and never loads on the
  login page. The QPS chart repaints at most once per ~1.5 seconds.
- **Global notices** name a DoH3/DoQ listener that is enabled but not listening,
  and upstream resolvers the health monitor considers degraded.
- **“Why was this blocked?”** appears under a blocked query-log row: the custom
  rule or blocklist source plus the matched pattern, visible on phones too.
- **Access-mode reporting fixed:** `/api/config/access` now returns the live
  mode instead of a stale copy. The redundant public-mode banner was removed.

## What's new in 5.4.2

**The Android Transports screen fits without scrolling.** This is an Android
UI patch; the resolver, API, database and transport behaviour are unchanged.

- On wide displays, DoH3 and DoQ sit **side by side**; on phones the cards
  stack in a much shorter layout.
- The listener status badge moved into each card header, the redundant intro
  card is now one muted line, and the separate authentication label is gone.
- The DoQ IP-allowlist summary, counts and Public Mode warning are one compact
  note.
- The SNI-token list is height-capped and scrolls inside the card.
- The standard IP-auth screen fits without scrolling on a 411 × 914 dp phone
  and a 1280 × 720 dp tablet.

## What's new in 5.4.1

**Safer transport changes, QUIC limits, and a DoQ access screen that explains
itself.** This is a patch on the 5.4 line; the DoH3/DoQ feature set is
unchanged.

- **DoH3 applies immediately**; **DoQ is staged behind Save DoQ** because its
  port, auth mode, token domain and token list must be valid together.
- Transport changes are **applied before they are persisted**. A failed bind
  rolls the listeners back, so the saved switch can never be ahead of what is
  actually serving. The failure reason stays on the card in `last_error`.
- DoH3 reports **listening only after the HTTP/3 serve loop starts**, and a
  listener that dies later is reported instead of looking healthy.
- **QUIC hardening:** per-source handshake rate limiting before TLS, a
  1,024-connection DoH3 cap, bounded receive windows, and new connection,
  protocol-error and handshake-rejection metrics.
- **DoQ IP auth is explicit:** the card names **Clients → Allowed IPs** and
  **Settings → Access → Server-wide Resolver Allowlist**, shows live counts,
  warns when Public Mode makes the list irrelevant, and warns when whitelist
  mode has no entries. Android has the same screen.
- **Upstream health check:** every five minutes the resolver re-probes each
  upstream and logs a warning when one is degraded, plus a recovery line.
- **Android:** DoH3 auto-save, staged Save DoQ, applied-versus-staged state,
  server error display, and the Save DoQ button shares the firewall row.

No database migration is required.

## What's new in 5.4.0

**Client-facing DoH3 and DoQ.** DigitDNS can now serve encrypted DNS to
clients over **HTTP/3** and **DNS-over-QUIC**, not just DoT and DoH. Both are
opt-in and off by default, so existing installs keep their current listeners
until an operator enables them.

- **DoH3 (HTTP/3)** listens on **UDP 8443** at the same `/dns-query` URL as
  DoH. TCP DoH advertises it with `Alt-Svc`, so capable browsers upgrade
  automatically; clients that cannot use QUIC fall back to TCP DoH.
- **DoQ (DNS-over-QUIC)** listens on **UDP 853** and implements RFC 9250:
  one query per stream, transaction ID 0, 2-byte length framing.
- **DoQ authentication** is either the existing client IP allowlist or an
  **SNI token** (`quic://<token>.doq.example.com:853`). Token mode requires a
  certificate covering the token hostnames; the built-in ACME client issues
  HTTP-01 certificates only, so a wildcard/multi-SAN pair must be supplied
  separately.
- **Settings → Client Encrypted Transports** is a new two-column card with
  live listener status, copyable endpoints, DoQ port/auth/token controls and a
  single Save action. The Connect Guide and subscriber portal expose the new
  endpoints, and Android gets a matching **Settings → Transports** screen.
- **Prometheus** adds `digitdns_doh3_requests_total`,
  `digitdns_doq_queries_total` and `digitdns_doq_auth_failures_total`.
- **Operations:** open **UDP 853** and **UDP 8443** in the firewall. The
  installer, firewall documentation and container `EXPOSE` list cover them.
- The SNI proxy cards now separate **actionable refusals** from **unreadable
  scanner traffic**, so a public port under scanning no longer looks like a
  failing relay.

No change is made to UDP/TCP, DoT or DoH behaviour. Both new transports are
disabled until enabled.

## What's new in 5.3.3

**The SNI proxy drop counters no longer count scanner noise.** A source that
keeps retrying while in the malformed-probe penalty box is counted as a flood
attempt, not as a relay drop. The web and Android cards now show actionable
refusals and unreadable connections separately from that noise, with the full
breakdown still available in the tooltip. Prometheus gains
`digitdns_relays_refused_actionable_total` and `digitdns_relays_flood_total`
alongside the existing total.

## What's new in 5.3.2

**Settings balances its cards and uses the wide layout.** Server Location now
grows to fill the left column, so it ends level with Server Update on both
systemd and Docker, and its title lines up with the update card's. On panels
1400 px and wider, Subscription Portal and the SSL/TLS Manager share a row: the
TLS card stretches to the taller card's height and its field rows reflow to two
columns with full-width action buttons.

## What's new in 5.3.1

**Settings uses its width better.** The Server Location card is a one-line
metadata strip (country and GeoIP database date together), the Two-Factor
Authentication card sits directly under it, and Server Update owns the right
column. The same layout works on systemd and Docker: the container image/binary
selector simply makes the update card taller.

## What's new in 5.3

**The panel is faster, the resolver is leaner, and Android starts better.**
The cached query path skips redundant EDNS/DNSSEC normalisation and reuses the
lowercase name the resolver already computed — the full cached-query benchmark
drops from ~932 ns to ~770 ns. The dashboard externalises its favicons, serves
every asset from a content-versioned immutable URL, and exposes Prometheus
metrics with a documented Grafana dashboard. The agent catalogue and heartbeat
paths now scale with payloads rather than per-agent database work.

On Android, a NavigationRail replaces the bottom bar at 600 dp and above, a Quick
Settings tile toggles ad blocking without opening the app, and generated baseline
and startup profiles are compiled into release builds (`assets/dexopt/baseline.prof`).
The About page keeps the update controls on the first screen, and Relay Agents
shows the applied control-plane revision and its age.

## What's new in 5.2

**DNSSEC validation is now built into the hub, the panel and the Android app.**
The resolver walks the DS/DNSKEY chain from the IANA root anchors, verifies
answer signatures and NSEC/NSEC3 denial proofs, sets `AD` only on answers it
validated itself, and answers `SERVFAIL` for bogus zones. `+cd` bypasses
validation. The feature is opt-in and startup-scoped: enable it in
**Settings → DNSSEC Validation** (web or Android), then restart the daemon.
The same release adds the v3 DNSSEC settings route, a compact Android card that
opens with the real saved state, shared status-pill styling, and DNSSEC-aware
cache keys so DO=0 clients never receive signature or denial records.

**5.2.13 removes the hub-restart stampede.** The agent control-plane revision is
now persisted and carries a build fingerprint, so a hub restart with unchanged
config answers 304 instead of forcing every agent to pull a full catalogue. The
rules and client list behind each catalogue are cached per revision, heartbeats
share one batched write, the agent's safety sync moved from 5 to 30 minutes, and
the agent's HTTP pool is explicit. New metrics expose catalogue requests, 304s,
rebuilds and parked long-polls.

**5.2.12 makes container updates a choice.** Docker installs can now pick the
update method in **Settings → Server Update**: *Pull image* (the default) keeps
the truthful compose pull/recreate path, while *Patch binary* re-enables the
in-container swap with signature verification, a persistent warning and a badge
that tells you when a recreate will restore the image's binary. The same
selector is available in the Android app, and the About page now fits the update
controls on one screen without scrolling.

**5.2.11 makes the panel and the phone faster and adds two operator tools.**
Assets are now served from content-versioned URLs with a one-year immutable
cache, so repeat visits stop revalidating every script and stylesheet. The
dashboard polls stats once a second only on the Dashboard tab. The CSP drops
`script-src 'unsafe-inline'` in favour of SHA-256 hashes for the inline boot
scripts. A Prometheus `/api/metrics` endpoint and a read-only rule tester
(web and Android) answer the two questions operators ask most: “how is the
resolver doing?” and “why is this domain blocked?”. On Android, ordinary API
calls get a finite read timeout again, and the upstream rows no longer carry
the transport/policy/last-error line.

**5.2.10 finishes the stuck-updater fix.** The progress modal now treats a
finished or restarted apply as terminal instead of polling an idle status
forever, and the status endpoint heals a stale `Running` flag even when the
operator does not click Update again. Container installs still refuse the
binary-swap path and show `docker compose pull && docker compose up -d`.

**5.2.9 fixes the web updater in Docker.** Container installs no longer offer the
binary-swap path — it replaced the container’s writable-layer binary but never
pulled the image the operator deploys. The panel now shows the correct
`docker compose pull && docker compose up -d` command. A stale apply or a restart
that did not exit now releases the “update in progress” lock instead of blocking
every later click. systemd installs keep the verified binary update.

**5.2.8 makes the web live log robust when SSE is buffered or blocked.** A REST
fallback reads the server’s recent-log ring, so the Log tab still fills behind a
proxy that buffers EventSource. The empty state now says whether it is connecting,
polling or listening, an expired session raises the login prompt, and Pause/Resume
no longer resizes the toolbar. The filter compile log now spells out exact and
suffix entries, which is why the rule count is about twice the unique-domain count.

**5.2.7 fixes the web live log.** The browser stream now stays connected while
you browse in another tab or another panel screen, so queries keep arriving in
the log instead of being dropped while the tab is hidden. If the daemon restarts
and the in-memory web session is gone, the panel now shows the login prompt
instead of stale tiles, and the stream reconnects after login. Reconnects are
single-flight, so a visibility change or a refresh cannot leave two streams open.

**5.2.6 hardens the request paths and makes the hub→agent control plane responsive.**
DNS, relay and upstream panics are contained instead of killing the daemon; the
upstream breaker has a half-open state, cached JSON is gzip-negotiated, and the
live query stream only opens on the Log tab. Agents long-poll the catalogue with
ETag/304, so rule and client changes normally arrive in under a second, the online
window is 90 seconds, and the agent relay caches target DNS, dials addresses in
parallel and tunes TCP. Malformed proxy probes now enter a per-source penalty box.

**5.2.5 makes the upstream health figures trustworthy and speeds up the hot paths.**
Latency is now the median of real transport round trips rather than goroutine
wall time, startup probes are counted separately, and the health dot follows the
recent error rate instead of lifetime failures. An unmeasured resolver shows `—`,
never a seeded 12 ms. Unmetered SNI relays now splice in the kernel, and live and
history stats are cached per process so several dashboards share one computation.

**5.2.4 stops malformed scanner probes from inflating the SNI Refused count.**
Public proxy ports constantly receive invalid SNI/Host probes. Those are now
counted separately (`relays_refused_malformed`) and shown in the Refused tooltip
as “not counted”; the card’s Refused number only moves for actionable refusals:
blocked/self targets, quota, a full relay table or a resolution failure.

**5.2.3 makes the resolver and the panel faster, and explains the drops.**
Upstream racing now defaults to Hedged, DoT and DoQ connections are reused, and
the history/settings fan-out was collapsed into far fewer requests. The SNI
proxy now reports why a connection was refused, upstream errors are split into
transport failures and policy rcodes, and a circuit breaker stops retrying a
dead resolver. Connection state and last errors stay in tooltips rather than
crowding the cards.

**5.2.2 adds optional IPv6 for proxied names and relay agents.** A hub-wide
**Proxy IPv6** switch and a per-agent **Use IPv6 for this agent** opt-in publish
AAAA only when both are on. The hub pushes the effective policy to agents on
their heartbeat; turning it off rebinds the agent IPv4-only, stops IPv6
detection and reporting, and keeps proxied AAAA withheld. Proxied
`HTTPS`/`SVCB` hints stay sunk even when proxy leak protection is off. The
Android app gains an **IPv6** Settings card, a per-agent switch, and compact
Relay Agents and Traffic History screens that fit a phone without scrolling.
Sign-in no longer flashes a stale “session expired” line.

The 5.1 line remains included: Graphite theme, reverse-DNS filtering in Top
Domains, Docker Hub mirrors, faster web/Android boot paths and the previous
performance work. See the latest release notes for details.

## What's new in 5.1

**Graphite** is a neutral third theme for the web panel and the Android app,
alongside the existing Dark and Light modes. One theme button cycles
Dark → Grey → Light, the choice is stored, and the selected theme carries
through login and restarts. The release also removes the cyan highlights from
buttons, chips, selected states and card edges, and replaces the launcher icon
with a grey mark.

The 5.0.1–5.0.4 patch line remains included: DNS Rewrites bulk import, Queries
by Relay, unified uppercase card headings, allowed-only Top domains, live-log
and locale fixes, the multi-arch agent setup selector, and real arm64 hub/agent
images. See the latest release notes for details.

### 🛡️ Ad blocking engine

DigitDNS is now a DNS sink as well as a resolver. Blocked names are answered
before they reach an upstream, with the policy and list management in the same
panel.

- **45 curated blocklists** in five groups: General protection, Supplementary ads
  & trackers, Malware/phishing/scams, Device & vendor telemetry, and Optional
  extras. Sources include HaGeZi Pro / Light / Pro+ / Ultimate, OISD Big / Small,
  AdGuard DNS, 1Hosts Xtra / Lite, StevenBlack Unified, Peter Lowe, AdAway,
  Dan Pollock, URLhaus and Phishing Army.
- **Install in one click.** A recommended base is marked, each preset shows its
  description, licence, warning, homepage and complete raw file, and its domain
  count is scanned in the background before you commit.
- **Custom lists.** Subscribe to any hosts/Adblock URL, or upload a local file.
  Downloads are capped at 64 MB, uploads at 16 MB, duplicate URLs are rejected and
  parsing runs off the DNS hot path.
- **Policy you control.** Enable/disable blocking, choose `0.0.0.0`, `NXDOMAIN`
  or `REFUSED`, set the block TTL and update interval, and keep an always-allow
  list that wins over every blocklist.
- **Active lists first.** Enabled lists are pinned to the top under an **ACTIVE**
  heading on web and Android and removed from their catalogue groups, so what is
  actually blocking is the first thing you see.
- **Per-list lifecycle.** Toggle without deleting, update one list or all lists,
  refresh on the configured schedule (default 24 hours, checked every 15
  minutes), delete, and see unique domains, rules, allow/skipped entries and the
  last update.
- **Web and Android UI.** A full **Ad Blocking** tab on web and **Settings → Ad
  Blocking** on Android, including custom URL and file upload.
- **API v3.** `/api/v3/filtering` exposes status, settings, presets, list
  creation, enable/disable, update and delete for automation.

### 📊 Aggregated statistics

- **Five ranges:** 1h, 24h, 7d, 30d and 90d.
- **1h draws from real five-minute buckets.** The current hour is split into
  twelve slices, so the chart has 10–12 points as the hour progresses instead of
  a single dot. 24h/7d read hourly buckets; 30d/90d read daily buckets.
- **Aggregate-only storage.** `stats.db` keeps counters and bounded top-N tables,
  never the raw query log. Hourly buckets roll into daily history; retention
  defaults to 90 days.
- **Distinct clients with HLL.** HyperLogLog gives an unbounded distinct count;
  pre-HLL ranges say `top-N approx.`.
- **Six ranked tables:** domains, blocked domains, clients, upstreams, protocols
  and relay countries, with client names and relay flags.
- **Privacy controls.** HMAC-SHA256 client identifiers, per-store salt and an
  opt-out for up to 500 IPs or account names.
- **Sticky toolbar and exports.** Refresh on the left, range chips in the middle,
  CSV/JSON on the far right in the action colour. The row stays pinned while
  cards scroll underneath.
- **Every history card moves.** History cards participate in the dashboard layout
  editor alongside the live cards.

### 🧩 Free dashboard layout

- Cards move anywhere on a **12-column, 12-pixel-row grid**, across the metrics,
  main and history groups, and resize from any edge or corner.
- Collision-safe drops, saved v3 layout with automatic migration, and a reset
  action beside the edit pencil.
- The six live KPI cards wrap their meta columns, CPU line and traffic readings
  instead of clipping at narrow widths.

### 📱 Android 5.0.0

- Range chips in the top header beside **Online**, with the aggregate update
  clock next to them.
- No timestamp flashing during background polls; the same clock appears in the
  same place on the other live-backed screens.
- 1h five-minute chart, numeric chart axis with gridlines, and the six-metric
  summary in two compact rows of three.
- Full **Settings → Ad Blocking** screen with the preset catalogue, custom URL
  and file upload, active-first ordering, green active states and list actions.

### 🧭 Also in the 4.x line

- **Geo relay agents.** A small agent in another country carries encrypted TLS
  byte-for-byte, with per-domain and per-service relay choices, a real agent
  page, manageable keys, connection tests and multi-arch images.

---

## Screenshots

### Web dashboard

Live metrics, the sticky history toolbar, the 1h range and export actions. Cards
can be moved and resized freely.

<p align="center">
  <img src="docs/screenshots/v5-web-dashboard.jpg" alt="DigitDNS web dashboard: live DNS metrics, 1h aggregate and CSV/JSON exports" width="100%">
</p>

### Web history

The 1h range drawn from five-minute buckets, with the summary, stacked traffic
chart and ranked tables.

<p align="center">
  <img src="docs/screenshots/v5-web-history.jpg" alt="DigitDNS web history: 1h five-minute chart, summary and top domains" width="100%">
</p>

### Web ad blocking

Forty-five curated blocklists, custom URL and file lists, an allowlist, and the
enabled lists pinned under **ACTIVE** at the top.

<p align="center">
  <img src="docs/screenshots/v5-web-adblocking.jpg" alt="DigitDNS web ad blocking: active blocklists, presets, policy and allowlist" width="100%">
</p>

### Android

| Dashboard | History | Ad blocking |
| :---: | :---: | :---: |
| <img src="docs/screenshots/v5-android-dashboard.png" alt="DigitDNS Android dashboard with range chips and updated clock" width="240"> | <img src="docs/screenshots/v5-android-history.png" alt="DigitDNS Android history summary and five-minute chart" width="240"> | <img src="docs/screenshots/v5-android-adblocking.png" alt="DigitDNS Android ad blocking with active blocklists" width="240"> |

### Graphite theme

The neutral third theme, shared by the web panel and the Android app. One
three-state button cycles **Dark → Grey → Light** and stores the choice, so the
theme chosen on the login screen survives sign-in and app restarts.

<p align="center">
  <img src="docs/screenshots/v51-web-graphite.jpg" alt="DigitDNS web dashboard in the Graphite theme" width="100%">
</p>

| Android login | Android dashboard | Android theme settings |
| :---: | :---: | :---: |
| <img src="docs/screenshots/v51-android-graphite-login.png" alt="DigitDNS Android login in the Graphite theme with the theme switch" width="240"> | <img src="docs/screenshots/v51-android-graphite-dashboard.png" alt="DigitDNS Android dashboard in the Graphite theme" width="240"> | <img src="docs/screenshots/v51-android-graphite-settings.png" alt="DigitDNS Android theme settings with Follow system, Dark, Grey and Light" width="240"> |

> Screenshots use a test server. The public address is replaced with a
> documentation address or pixelated; no client names are shown.

---

## Why DigitDNS

- **One binary, no runtime.** DNS, DoT, DoH, the TLS relay, the panel, the REST
  API, the subscriber portal and the self-updater are compiled into
  `digitdns`. The only state is a local encrypted database, a master key and a
  JSON config file.
- **Rules per domain, not a mystery blocklist.** Proxy, block or route direct,
  with wildcard scopes you can see (`exact`, `*.host`, `*.parent`), named
  groups, bulk import and one searchable Domains list.
- **A relay that never decrypts.** Proxied HTTPS is forwarded byte-for-byte.
  The server never holds the session keys or the plaintext.
- **Built for paying subscribers.** Per-client quotas, expiry, device limits,
  traffic accounting, a private portal link, a registration secret and an
  Android client area.
- **Encrypted DNS where it matters.** DoT, DoH, DoH3, DoQ and DNSCrypt for clients; DoT, DoH, DoQ, DoH3 and DNSCrypt for upstreams.
- **DNSSEC validation when you want it.** Optional chain-of-trust validation
  with built-in root anchors, NSEC/NSEC3 denial proofs, `AD` on validated answers
  and `SERVFAIL` for bogus zones.
- **IPv6 when you want it.** Publish your own IPv6 for proxied names globally or
  per relay agent; off keeps clients on IPv4 and withholds AAAA and IPv6 hints.
- **A Pi-hole / AdGuard Home alternative with a relay.** Run the 45-preset
  blocklist engine on its own, or let the same install bypass geo-blocks through
  SNI relay agents.

**What it is not:** a VPN. Only names you mark as proxied are relayed; all other
traffic keeps its direct path. It also does not hide your server's IP from the
destination of a proxied connection — that is the point of the relay.

<p align="center">
  <img src="docs/digitdns-architecture.png" alt="Devices, the DigitDNS resolver/relay, upstreams and proxied destinations" width="100%">
</p>

---

## Quick start

You need a Linux server (1 vCPU / 1 GB is enough for personal use), root
access, and one hostname whose **A record points at the server**. The panel is
HTTPS-only and obtains its own Let's Encrypt certificate, so the domain is not
optional for a normal install.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.2/scripts/install.sh | sudo bash
```

The installer sets up the service, firewall, certificate, admin password and a
random panel port/path, then prints the login URL. Open it, sign in, and point a
device at the server's IP on port 53 (UDP and TCP).

Verify from another machine:

```bash
dig @YOUR_SERVER_IP example.com A
```

> [!IMPORTANT]
> Save the admin password and back up `master.key` from the data directory.
> The key encrypts every subscriber record; without it the database cannot be
> read. See [Backup & restore](#backup--restore).

---

## Requirements

| | Minimum | Comfortable |
| :--- | :--- | :--- |
| CPU / RAM | 1 vCPU, 1 GB | 2 vCPU, 2 GB (~10 active subscribers) |
| Disk | 1 GB | 5 GB (logs, certificates, database growth) |
| OS | Ubuntu 20.04+, Debian 11+, AlmaLinux/Rocky 8+ (systemd) | Ubuntu 22.04+ / Debian 12+ |
| Network | A public IPv4; IPv6 optional | Static IP, port 80 reachable for ACME |
| Domain | One hostname → server A record | A second hostname for DoH/DoT is optional |

**Ports**

| Port | Protocol | Purpose |
| :--- | :--- | :--- |
| 53 | UDP + TCP | Plain DNS for clients |
| 80 | TCP | ACME HTTP-01 challenge and the HTTP relay |
| 443 | TCP | HTTPS relay for proxied domains |
| 853 | TCP | DNS-over-TLS (when TLS is configured) |
| 853 | UDP | DNS-over-QUIC (opt-in) |
| 8443 | TCP | DNS-over-HTTPS at `/dns-query` (when TLS is configured) |
| 8443 | UDP | DoH over HTTP/3 (opt-in) |
| 5443 | UDP + TCP | DNSCrypt (opt-in; avoids TCP 443, which the SNI proxy owns) |
| random 20000–60000 | TCP | Web dashboard; chosen at install and printed in the URL |

**Relay agent (optional)**

An agent needs a public IPv4, inbound TCP **80/443**, outbound HTTPS to the hub,
and no domain or certificate. It is Linux amd64/arm64. The hub keeps all other
requirements. Full manual: [docs/AGENT.md](docs/AGENT.md).

DNSCrypt, DoQ and DoH3 are also **upstream transports**. As client listeners
they are opt-in under **Settings → Client Transports**, together with
DoT SNI-token authentication and mTLS. Open the same ports in the firewall.

---

## Installation

### 1. One-line installer (Linux, recommended)

Interactive:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.2/scripts/install.sh | sudo bash
```

Non-interactive:

```bash
DIGITDNS_DOMAIN=dns.example.com \
DIGITDNS_EMAIL=you@example.com \
bash install.sh
```

Optional environment variables:

| Variable | Meaning |
| :--- | :--- |
| `DIGITDNS_DOMAIN` | Panel hostname (must already point at the server) |
| `DIGITDNS_EMAIL` | Let's Encrypt account email |
| `DIGITDNS_PUBLIC_IP` | Override the detected public IPv4 |
| `DIGITDNS_FRESH=1` | Confirm a non-interactive fresh install / reinstall |

The installer supports amd64 and arm64, writes a systemd unit, opens the
required firewall ports, installs `restore.sh` and `uninstall.sh`, and archives
an existing install before upgrading it. Everything lives under
`/opt/digitdns`.

**Offline install:** download `digitdns-offline-bundle.tar.gz` (or `.zip`) from
the release page, copy it to the server, then run `install.sh` from the
extracted directory. The offline bundle is Linux amd64; it needs internet only
for the first certificate issuance.

### 2. Docker

Images are published for amd64 and arm64 and signed with cosign. The primary
registry is GHCR: `ghcr.io/digitrob/digitdns` (hub) and
`ghcr.io/digitrob/digitdns-agent` (optional relay agent, see
[Relay agent](#6-relay-agent-optional)). Docker Hub mirrors the same tags at
`robmeister/digitdns` and `robmeister/digitdns-agent`.

```bash
docker pull ghcr.io/digitrob/digitdns:latest
# Docker Hub mirror:
# docker pull robmeister/digitdns:latest

mkdir -p data certs
docker run -d --name digitdns --network host \
  -v "$PWD/data:/app/data" -v "$PWD/certs:/app/certs" \
  -e TZ=UTC \
  ghcr.io/digitrob/digitdns:latest
```

Or with Compose:

```yaml
services:
  digitdns:
    image: ghcr.io/digitrob/digitdns:latest
    container_name: digitdns
    restart: always
    network_mode: "host"
    volumes:
      - ./data:/app/data
      - ./certs:/app/certs
    environment:
      - TZ=UTC
    ulimits:
      nofile: { soft: 65535, hard: 65535 }
```

Set `image: robmeister/digitdns:latest` instead if you prefer the Docker Hub
mirror.

```bash
mkdir -p data certs && docker compose up -d
docker logs -f digitdns
```

**Host networking is required**, not a preference: the resolver must see real
client addresses for whitelisting, subscriber recognition and rate limiting,
and DNS/DoT/DoH are not simple `-p` mappings.

| Tag | Meaning |
| :--- | :--- |
| `latest` | Latest stable release |
| `5.5.2` | Warm benchmark medians, auto-benchmark on add, stable live latency, upstream UI fixes (latest) |
| `5.5.1` | DoQ FIN fix, offline installer firewall sync; client transports, DNSCrypt, mTLS and UI polish (older) |
| `5.5.0` | Client Transports with DNSCrypt, mTLS and token auth; live DNSSEC apply; steadier web/Android panels (older) |
| `5.4.3` | API gzip, minified assets, deferred chart, health/blocked notices (older) |
| `5.4.2` | Compact adaptive Android Transports screen; no server changes (older) |
| `5.4.1` | Transport hardening, QUIC limits/metrics, DoQ allowlist clarity, upstream health check (older) |
| `5.4.0` | Client-facing DoH3 and DoQ (opt-in), web/Android transport controls, SNI-token auth, QUIC metrics (older) |
| `5.3.3` | Accurate SNI proxy drop counters; scanner flood attempts excluded from web and Android headlines (older) |
| `5.3.2` | Balanced Settings cards; Server Location fills the left column and Subscription Portal pairs with the SSL/TLS Manager on wide panels (older) |
| `5.3.1` | Compact Server Location and 2FA under it in Settings; works on systemd and Docker (older) |
| `5.3.0` | Faster cached query path, metrics docs + Grafana, adaptive Android navigation, Quick Settings tile, baseline profiles (older) |
| `5.2.13` | Persisted agent revision, shared catalogue cache, batched heartbeats, faster reconnect, control-plane metrics (older) |
| `5.2.12` | Container update method selector (web + Android), temporary-swap marker and badge, compact Android update page (older) |
| `5.2.11` | Immutable asset caching, tab-aware stats polling, CSP script hashes, Prometheus metrics, rule tester on web and Android (older) |
| `5.2.10` | Update modal reaches a terminal state and the status endpoint heals a stale Running flag (older) |
| `5.2.9` | Docker updater refuses binary swap and shows the image pull/recreate command; stale apply lock fixed (older) |
| `5.2.8` | Web live log REST fallback, clearer empty state, fixed Pause/Resume toolbar and exact/suffix filter log (older) |
| `5.2.7` | Web live log stays connected while browsing; session-expiry prompt and single-flight reconnect (older) |
| `5.2.6` | Panic containment, half-open upstream breaker, gzip JSON, hub↔agent long-poll and agent relay tuning (older) |
| `5.2.5` | Trustworthy upstream metrics, SNI relay splice and shared stats caches (older) |
| `5.2.4` | SNI Refused counts actionable refusals only; malformed scanner probes are reported separately (older) |
| `5.2.3` | Faster resolver and panel, Hedged upstreams, DoT/DoQ reuse, one-request history, SNI drop reasons and upstream circuit breaker (older) |
| `5.2.2` | Optional IPv6 for proxied names and relay agents, Web/Android IPv6 controls, silent re-auth and compact Android cards (older) |
| `5.2.1` | Proxy leak protection for proxied HTTPS/SVCB, live panel/Android control, login and refresh UI fixes (older) |
| `5.2.0` | DNSSEC validation, Settings card, v3 API and Android DNSSEC section (older) |
| `5.1.4` | Reverse DNS excluded from Top Domains; Docker Hub mirror published (older) |
| `5.1.3` | Faster web route boot and Android startup, lifecycle and live-log batching (older) |
| `5.1.2` | Backend performance: telemetry fast path, compiled policy indexes, typed DNS answers (older) |
| `5.1.1` | Sky-blue Proxy status in Graphite for web and Android, grey favicon (older) |
| `5.1.0` | Graphite theme for web and Android, neutral controls, grey app icon (older) |
| `5.0.4` | Real arm64 hub/agent images and arm64 execution CI gate (older) |
| `5.0.3` | Multi-arch agent selector, copyable Docker command, arm64 CI gate (older) |
| `5.0.2` | Allowed-only Top domains, locale-safe UI, Rewrite filter and import polish (older) |
| `5.0.1` | Dashboard polish, DNS Rewrites, relay labels, unified cards (older) |
| `5.0.0` | Ad-blocking engine, aggregate stats, free dashboard layout (older) |
| `4.1.0` | 90-day traffic history, Android history cards (older) |
| `4.0.3` | Dashboard refresh, compact Settings, app-icon brand (older) |
| `4.0.2` | Accurate client-list status (older) |
| `4.0.1` | Agent log-throttling patch (older) |
| `4.0.0` | Geo relay agents (older) |
| `3.8.0` | A specific immutable release (older) |
| `edge` | Nightly build from `main` — not for production |

Verify the image:

```bash
cosign verify ghcr.io/digitrob/digitdns:latest \
  --certificate-identity-regexp 'https://github.com/digitrob/DigitDNS/.github/workflows/docker.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The Docker Hub mirror carries the same signature and digest:

```bash
cosign verify docker.io/robmeister/digitdns:latest \
  --certificate-identity-regexp 'https://github.com/digitrob/DigitDNS/.github/workflows/docker.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

### 3. Pre-built binary + systemd

Release binaries are Linux amd64 and arm64. Checksums are published as
`checksums.txt` and signed as `checksums.sig`.

```bash
curl -fsSL -o digitdns \
  https://github.com/digitrob/DigitDNS/releases/download/v5.5.2/digitdns-linux-amd64
chmod +x digitdns
sudo install -m 0755 digitdns /opt/digitdns/digitdns
```

Use the canonical unit from `scripts/digitdns.service` (or adapt it):

```ini
[Unit]
Description=DigitDNS
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/digitdns
RuntimeDirectory=digitdns
RuntimeDirectoryMode=0700
ExecStart=/opt/digitdns/digitdns -daemon \
  -config /opt/digitdns/config.json \
  -db /opt/digitdns/data.db \
  -key /opt/digitdns/master.key
Restart=always
RestartSec=3s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Start with `./digitdns -server` on a fresh machine to generate the database,
key, config and first-run credentials; the service runs `-daemon`.

### 4. Build from source

Requires Go 1.26+.

```bash
git clone https://github.com/digitrob/DigitDNS.git
cd DigitDNS
go build -ldflags="-s -w" -o bin/digitdns ./cmd/digitdns
./bin/digitdns -server
```

The Go daemon builds on other operating systems for development, but the
installer, systemd unit and packaged binaries target Linux.

### 5. Android app

The companion APKs are attached to every release. See
[Android app](#android-app) for the architecture table and installation steps.

#### Baseline profiles

The Android app ships generated baseline and startup profiles
(`android/app/src/release/generated/baselineProfiles/`), which are compiled into
`assets/dexopt/baseline.prof` in release builds. Regenerate them after a
navigation or startup-path change with a connected API 33+ device or emulator:

```bash
cd android
./gradlew :app:generateBaselineProfile \
  -Pandroid.testInstrumentationRunnerArguments.androidx.benchmark.suppressErrors=EMULATOR
```

The generator drives a cold start through the first rendered screen.

### 6. Relay agent (optional)

An agent is a second, much smaller process on a public IPv4 in the country you
want to exit from. It needs no domain, certificate, database or panel; only
outbound HTTPS to the hub and inbound TCP **80/443** for relayed traffic.

1. On the hub open **Settings → Relay Agents → Add agent**, choose the country,
   and copy the key shown after saving.
2. Run the container on the agent VPS:

```bash
mkdir -p /opt/digitdns-agent
docker run -d --name digitdns-agent --network host --restart unless-stopped \
  -v /opt/digitdns-agent:/data \
  ghcr.io/digitrob/digitdns-agent:latest \
  -hub 'https://panel.example.com:8080/ADMIN_PATH' \
  -key 'AGENT_ID:SECRET' \
  -listen :80,:443 \
  -state /data/agent-state.json
```

Host networking is required: the agent must see the real client address and
bind 80/443 without port remapping. The `-hub` value is the same panel origin
you use in a browser, including the hidden admin path.

3. Back on the hub, wait for the card to show **online**, press **Test
connection**, then assign domains or a service default to the agent.

The full manual — binary and systemd install, every flag, key rotation,
troubleshooting, sizing and security — is in [docs/AGENT.md](docs/AGENT.md).

### Migrating from HyperDNS

The installer detects `/opt/hyperdns`, archives it and migrates the data in
place. For a HyperDNS 1.x install, run `migrate-from-v1.sh` from the offline
bundle first.

---

## First run

On a fresh install the daemon generates:

- an admin password, printed **once**;
- a hidden 16-character admin path;
- a dashboard port between 20000 and 60000;
- `master.key`, `data.db` and `config.json` under `/opt/digitdns`.

The panel is at:

```
https://YOUR_DOMAIN:PANEL_PORT/ADMIN_PATH/login
```

The bare hostname intentionally shows nothing, so scanners cannot discover the
panel. Bookmark the full URL.

**Where things live**

| Path | What it is |
| :--- | :--- |
| `/opt/digitdns/data.db` | Encrypted subscriber/rule database |
| `/opt/digitdns/master.key` | Key that decrypts the database — back this up |
| `/opt/digitdns/config.json` | Server, DNS, relay, access and TLS settings |
| `/opt/digitdns/certs/` | Certificates |
| `/opt/digitdns/backups/` | Archives created by the installer |

Run `digitdns` with no arguments over SSH for the text console: status,
diagnostics, restart/stop, subscriber management, upstream benchmark, cache
flush, panel port, API key rotation, admin recovery and uninstall.

---

## Features

### Domains and routing

One Domains list holds proxied, paused, blocked and direct names, with status
chips, search, sort and bulk selection. You can change a row's routing from its
status badge without losing its place in the list.

- **Wildcard scopes you can see:** `exact` (host and subdomains), `*.host`
  (subdomains only), `*.parent` (the parent's subtree). Changing a scope
  rewrites the rule in place and keeps its row position.
- **Policy groups** bundle domains into named, enable/disable-able sets with
  optional icons.
- **Bulk import** accepts pasted or uploaded lists with `#` comments and
  `*.wildcards`.
- Built-in direct routing keeps a handful of high-bandwidth services (voice
  chat, game-store CDNs, OS update CDNs) off the relay.

### Services

Services are labels, not routing rules. A domain can belong to one service, and
selecting that service in the sidebar filters the Domains table down to it.

- Create a service with a name and an optional icon.
- Assign it from the row's **Service** control, or select several domains and
  use **Assign service**.
- Type a domain in the icon field to use its favicon, paste an image URL, or
  search a word such as `tv` or `music` for a colourful icon.
- Rename or delete a service without touching the domains themselves. Deleting
  clears the label; it never changes proxy, block or direct status.

The Services panel stays pinned while a long domain list scrolls, and the whole
page now uses the width of a wide monitor instead of stopping at a narrow
column.

### Dashboard

The dashboard is a live view, not a static report: QPS, cache hits, memory,
CPU, relay connections, bandwidth, latency percentiles and the current upstream
resolvers.

- **Arrange it yourself.** The pencil beside **Dashboard** in the sidebar turns
  on edit mode. Drag a card anywhere on the 12-column, 12-pixel-row grid and
  resize it from any edge or corner. Click the struck-through pencil to leave
  edit mode; the layout is saved on the server.
- **The same layout everywhere.** The arrangement is stored install-wide, so it
  is there in the next browser and on the next device.
- **Phones reorder too.** Long-press a card to pick it up. Sizes stay at the
  normal mobile stacking, because a resized card on a phone is rarely useful.

### Ad blocking

The DNS sink sits in front of upstream resolution and answers blocked names
directly.

- **Curated and custom lists.** 45 presets plus URL subscriptions and local file
  uploads in hosts or Adblock format.
- **Policy.** Enable/disable, `0.0.0.0` / `NXDOMAIN` / `REFUSED`, block TTL,
  update interval and an always-allow list.
- **Active first.** Enabled lists are pinned at the top of the web and Android
  list views and removed from their catalogue groups.
- **Per-list control.** Toggle, update, refresh all and delete, with unique
  domain, rule, allow, skipped and last-update counts.
- **Scheduling.** Lists refresh in the background on the configured interval; the
  DNS path only reads the compiled snapshot.
- **API.** Full management through `/api/v3/filtering`.

### Traffic history

The optional 90-day aggregate store answers “what did this server see last
week?” without retaining the queries themselves.

- **Aggregate-only storage.** A separate `stats.db` holds counters and bounded
  top-N tables; the raw query log remains an in-memory ring of 100 entries and
  is never written to disk.
- **Retention you control.** 7 days of hourly buckets roll into daily buckets,
  which default to 90 days. Top-N limits, retention and anonymization are edited
  under **Settings → Traffic History** and apply after a daemon restart.
- **Distinct clients with HLL.** HyperLogLog registers give an unbounded distinct
  count; the UI says `HLL distinct` and only falls back to `top-N approx.` for
  ranges that contain pre-HLL buckets.
- **Privacy options.** Client anonymization replaces the identifier with an
  HMAC-SHA256 keyed by a per-store salt. A per-client opt-out accepts up to 500
  IPs or account names and suppresses the identifier while still counting the
  query in totals.
- **Dashboard and API.** Range selector (1h / 24h / 7d / 30d / 90d), summary,
  stacked chart and six ranked tables on web and Android; CSV/JSON export on web;
  `stats/summary`, `stats/series`, `stats/top`, `stats/export` and
  `stats/settings` in API v3.
- **Five-minute 1h series.** The 1h range is split into twelve five-minute
  buckets so the graph draws through the hour; 24h and 7d use hourly buckets and
  30d/90d use daily buckets.
- **Names, not addresses.** Ranked client rows resolve the stored address to the
  client’s current name through the access index; an unknown or deleted client
  keeps its IP. Relay rows show the country flag and localized name.

### Monitoring

`GET /<admin-path>/api/metrics` exposes a Prometheus endpoint covering the
resolver, cache, relay, latency and the agent control plane. See
[docs/METRICS.md](docs/METRICS.md) for the scrape config, the full metric
reference, PromQL examples and an importable Grafana dashboard.

### Proxy relay

Domains marked as proxied resolve to your server. The device opens TLS to the
server, which forwards the encrypted stream to the real host and back. No
decryption, no certificate substitution, up to 4,096 concurrent relays, with
optional TLS fragmentation for DPI resistance.

### Geo relay agents

An agent is an optional egress point in another country. The hub still resolves
the domain and enforces the client allowlist; the agent only carries the
encrypted stream. That separation is deliberate: a box in a foreign datacentre
holds no subscriber database and no panel credential.

**How a domain chooses its exit**

1. A domain override wins: **This Server**, a named agent, or **Inherit**.
2. With no override, the service's **Default relay** applies.
3. With no service default either, the local server is used.

**Setting it up**

- Add the agent on the hub, choose its country and copy the one-time key.
- Install the agent with Docker or the release binary. Keep the key out of shell
  history (use a file or an environment manager) and give it to the agent once.
- Open TCP 80/443 on the agent and allow outbound HTTPS to the hub. No inbound
  admin port is opened and no TLS certificate is installed on the agent.
- Assign domains from the row's relay control, set a service default in the
  service editor, or use bulk **Egress** for a selection.
- Press **Test connection** on the card. It performs a real TCP connect to the
  advertised relay port and records the latency; the badge is green under
  40 ms, amber above 40 ms and red above 80 ms.

**Operating an agent**

- The card shows last heartbeat, version, assigned domains, traffic and the
  services using it. An agent is online when its heartbeat is under two minutes
  old.
- **Rotate key** invalidates the old credential immediately. Update the agent's
  key and restart it. A key created before encrypted copy storage existed must
  be rotated once before the card can copy it again.
- Disabling or deleting an agent stops its relay immediately. Assignments fall
  back to This Server; delete removes those assignments from both the domain and
  service maps.
- Agents are IPv4-only. No agent certificate or agent domain is required for
  relay traffic; the client's TLS session is passed through untouched.
- Update an agent the same way you update the hub: pull the new image or replace
  the binary, then restart. Keep `/data/agent-state.json` — it holds the key and
  the last catalogue.

Full manual: [docs/AGENT.md](docs/AGENT.md).

### Subscribers and quotas

- Per-client traffic quota, reset cycle, expiry, device limit, notes and policy
  set.
- Whitelist mode by default: unknown source addresses are refused.
- Per-client traffic is metered through the relay and persisted every 30
  seconds.
- Registration secret lets a subscriber rebind their IP without an admin.
- Copy-ready provisioning card and subscription link for Telegram or your own
  bot.

### Client area

A subscriber gets a private link (`https://your-server/sub/<token>`) and, on
Android, the same surface natively:

<p align="center">
  <img src="docs/digitdns-client-area.png" alt="Subscriber portal: registration, usage, endpoints and setup guide" width="70%">
</p>

- **Status** — account state, detected IP, traffic, quota, reset and expiry;
  Registration Secret and Registered Address controls at the bottom.
- **Connect** — every registered address and subnet, plus DNS, DoH and DoT
  endpoints with copy buttons.
- **Guide** — complete setup steps for PlayStation, Xbox, Switch, Windows,
  iOS/macOS, Android and routers, using the live values for that subscription.
- **Saved subscriptions** — the app keeps a subscription with its account name
  above the admin servers, with one-tap reopen and **Switch to Admin**.

### Encrypted DNS

| Transport | Direction | Where |
| :--- | :--- | :--- |
| DoT | Clients and upstreams | Client listener on 853/TCP; upstream `tls://host` |
| DoH | Clients and upstreams | Client listener on 8443/TCP `/dns-query` (HTTP/2); upstream `https://host/dns-query` |
| DoQ | Clients and upstreams | Client listener on 853/UDP (opt-in); upstream `quic://host` |
| DoH3 | Clients and upstreams | Client listener on 8443/UDP (opt-in, advertised with `Alt-Svc`); upstream `h3://host/dns-query` |
| DNSCrypt | Clients and upstreams | Client listener on 5443/UDP+TCP (opt-in); upstream `sdns://…` stamp |

Client DoT/DoH need a valid certificate, so they require a domain. The server
keeps upstream connections alive between queries to cut handshake latency.

Client-facing **DoQ**, **DoH3** and **DNSCrypt** are opt-in under **Settings →
Client Transports**. DoQ and DoH3 share the DoH/DoT certificate and
the same access allowlist; open **UDP 853**, **UDP 8443** and **5443/UDP+TCP**
in the firewall. DoQ and DoT can authenticate by client IP or by an SNI token
(`quic://<token>.doq.example.com:853`, `tls://<token>.dot.example.com:853`);
SNI-token mode requires a certificate covering the token hostnames, and the
built-in ACME client issues HTTP-01 certificates only, so a wildcard/multi-SAN
pair must be supplied separately. mTLS can additionally require a client
certificate on DoT, DoH and DoQ.

### DNSSEC validation

DigitDNS can validate the DNSSEC chain of trust itself instead of trusting an
upstream resolver's `AD` bit. Validation is opt-in and startup-scoped.

- **Root to answer.** Walks DS/DNSKEY records from the built-in IANA root KSKs
  (`20326`, `38696`) through each delegation, then verifies the answer RRSIGs.
- **Positive and negative answers.** Verifies NXDOMAIN and NODATA with NSEC or
  NSEC3 proofs, including wildcard expansion and NSEC3 opt-out spans.
- **Honest AD.** `AD` is set only on answers this server validated. Bogus zones
  return `SERVFAIL` with Extended DNS Error 6. `CD=1` bypasses validation and
  clears `AD`; unsigned delegations are served normally without `AD`.
- **Cache-safe.** Cache entries separate EDNS/DO/CD, DO=0 clients never receive
  RRSIG/NSEC/NSEC3 material, background refresh keeps the entry's flags, and
  TTLs cannot outlive the nearest RRSIG expiration.
- **Panel and Android controls.** **Settings → DNSSEC Validation** on both
  surfaces: enable/disable, trust-anchor editor, built-in root anchors and a
  restart-required notice when saved policy differs from the running process.
- **API.** `GET/POST /api/settings/dnssec` for the panel and
  `GET/POST /api/v3/settings/dnssec` for Android and API-key clients.

```json
"dnssec_validation": true,
"dnssec_trust_anchors": []
```

An empty `dnssec_trust_anchors` list uses the built-in root KSKs. Custom entries
are DS or DNSKEY records for `.` and are validated before saving. Enabling or
changing the policy takes effect after a daemon restart; the default is `false`.

### Proxy leak protection

A proxied name must not advertise QUIC or the origin's real addresses. An
`HTTPS`/`SVCB` answer can carry an `h3` ALPN and `ipv4hint`/`ipv6hint` records;
either one lets a browser bypass the SNI relay. For names routed through
**Proxy**, the hub answers `HTTPS` and `SVCB` locally with `NOERROR`, no answers,
no `AD` and no cache entry. Direct names keep their normal records, and blocked
names already return the block response for every query type.

- **Live and on by default.** The `proxy_leak_protection` setting applies to the
  running resolver immediately; turning it off is an explicit opt-out.
- **Panel and Android controls.** **Settings → Proxy Leak Protection** on the
  web, and **Settings → DNSSEC/Proxy Leak** on Android. Disabling asks for
  confirmation and keeps the warning visible while protection is off.
- **API.** `GET/POST /api/settings/proxy-leak` for the panel and
  `GET/POST /api/v3/settings/proxy-leak` for Android and API-key clients.

```json
"proxy_leak_protection": true
```

### IPv6 for proxied names

IPv6 is optional and off by default. Proxied AAAA is answered only when the
server-wide **Proxy IPv6** switch is on, `public_ipv6` is set, and — for traffic
through a relay agent — that agent's own IPv6 switch is on.

- **Global master switch.** `proxy_ipv6_enabled` plus `public_ipv6` in Settings.
  Direct names keep their normal AAAA; proxied names receive the configured
  address.
- **Per-agent opt-in.** `ipv6_enabled` and `public_ipv6` on each relay agent.
  The effective policy is the agent's switch **and** the global switch.
- **Applied live, without a restart.** The hub sends the effective policy in the
  agent's catalogue and heartbeat reply; the agent rebinds within 30 seconds.
- **No leak when off.** Proxied AAAA is withheld, the agent listens IPv4-only and
  stops detecting/reporting IPv6, and proxied `HTTPS`/`SVCB` hints stay sunk
  even if proxy leak protection is disabled.
- **Panel and Android.** **Settings → Proxy IPv6** on the web; **Settings → IPv6**
  on Android. The relay-agent editor exposes the per-agent switch on both.

```json
"public_ipv6": "2001:db8::1",
"proxy_ipv6_enabled": true
```

### Upstream resolvers and policy

- Plain UDP/TCP, DoT, DoH, DoQ, DoH3 and DNSCrypt upstreams, benchmarked live
  from your server.
- Global **Default upstream**, **Strategy** and **Transports** policy.
- Strategies: **Fastest Wins**, **In Order**, **Hedged** (next resolver after
  50 ms) and **Best Known** (fastest measured, then latency order on failure).
- Transport policies: **Any Transport**, **Encrypted Only**, **Regular DNS** —
  enforced, not advisory.
- Per-domain overrides from the Domains gear; failures fall back to the pool.

### Certificates and diagnostics

- Embedded ACME client: issues and renews the panel certificate, plus an
  optional DoH/DoT certificate, without certbot or cron.
- Diagnostics with operator-defined targets, a reachability score and
  per-target latency.
- Server location from a self-updating GeoIP database, and in-panel update
  checks with signature verification.
- REST API v2/v3 with Swagger docs at
  `https://YOUR_DOMAIN:PANEL_PORT/ADMIN_PATH/api/v3/docs`.

---

## A look at it

<p align="center">
  <img src="docs/digitdns-tour.gif" alt="A short tour: dashboard, domains, clients and the subscriber portal" width="100%">
</p>

| Domains | Clients |
| :---: | :---: |
| <img src="docs/digitdns-domains.png" alt="Domains list with wildcard scopes and status filters"> | <img src="docs/digitdns-clients.png" alt="Client cards with quotas, registered IPs and provisioning actions"> |

## Android app

The Android app is a native companion with the same control surface as the
panel. It has its own version line (`android-v5.5.2`) and is side-loaded; it is
not on Play.

| APK | Architecture |
| :--- | :--- |
| `digitdns-arm64-v8a.apk` | 64-bit ARM — almost every phone since ~2016 |
| `digitdns-armeabi-v7a.apk` | 32-bit ARM — older devices |
| `digitdns-x86_64.apk` | Emulators and some tablets |
| `digitdns-universal.apk` | All of the above, larger |

Download from the [latest release](https://github.com/digitrob/DigitDNS/releases/latest)
and verify against the matching `.sha256`. Requires **Android 10 (API 29)** or
newer.

It covers the dashboard, Domains and wildcards, Clients and quotas, the live
query log with quick actions, upstream resolvers and policies, diagnostics,
certificates, settings and server switching (multiple saved servers, saved
credentials, API keys with an optional **Save key** tick). It also has the
subscriber client area.

On the Domains screen, long-press a row to start a selection. The selection bar
keeps Pause, Resume, Remove, **Assign service** and **Egress** on one line, and
the service filter uses the same labels and icons as the web panel. The second
line of a proxied row shows an icon-only service mark and relay flag; tapping
the service mark opens the service picker, and tapping the flag opens the relay
picker without moving the row's gear, status pill or switch.

**Settings → Relay Agents** is the mobile agent catalogue. Cards show the public
IP and masked key with copy buttons, a rotate-key action, version, last seen,
assigned domains, traffic and service defaults. **Test connection** opens a real
TCP connection to the relay port and updates the ping badge. The layout is one
card wide on a phone and two side by side on a tablet or in landscape.

**Traffic History** is part of the same dashboard: range switching, six KPI
tiles, the stacked traffic chart, top domains/blocked/clients/upstreams/
protocols/relay countries, flags and client names. Ranked cards are fixed-height
with five or three visible rows and internal list scrolling. **Settings →
Traffic History** edits the server-wide retention, top-N, anonymization and
client opt-out policy.

The app keeps its own version, so a server update never forces an app update.

<p align="center">
  <img src="docs/digitdns-android-status.png" alt="Android client area: Status with account, Device IP, traffic and registration" width="42%">
  &nbsp;&nbsp;
  <img src="docs/digitdns-android-connect.png" alt="Android client area: registered addresses and endpoints" width="42%">
</p>

---

## Operations

### Upgrade

- **Panel:** Settings → Server Update checks for a signed release and installs
  it with a backup and restart.
- **Docker:** `docker pull ghcr.io/digitrob/digitdns:latest && docker compose up -d`
  (or `docker pull robmeister/digitdns:latest` for the Docker Hub mirror).
- **Binary:** replace `/opt/digitdns/digitdns`, then `systemctl restart digitdns`.
- **Installer:** rerun it; the existing install is archived first.
- **Relay agents:** pull the new `ghcr.io/digitrob/digitdns-agent` image (or
  `robmeister/digitdns-agent` from Docker Hub, or replace the binary) and
  restart. `/data/agent-state.json` survives; it holds the agent key and last
  catalogue.

Back up before any upgrade. Database and config format changes are handled
automatically, but `master.key` must travel with `data.db`.

### Backup & restore

Stop the service before copying the database for a consistent snapshot:

```bash
sudo systemctl stop digitdns
sudo tar -czf /root/digitdns-backup-$(date +%F).tar.gz \
  -C /opt/digitdns data.db master.key config.json certs
sudo systemctl start digitdns
```

Keep the archive off the server. Restore with the service stopped by putting the
four items back, or use `scripts/restore.sh` for an archive produced by the
uninstaller. Never restore an old `master.key` into a fresh database — it will
not decrypt it.

### Logs

```bash
sudo journalctl -u digitdns -f          # systemd install
docker logs -f digitdns                 # hub container
docker logs -f digitdns-agent           # relay agent container
```

### Troubleshooting

| Symptom | Check |
| :--- | :--- |
| Blank page or 404 at the hostname | You need the full hidden URL with the panel port and admin path |
| Certificate issuance fails | A record points at this server; port 80 is open; Let's Encrypt rate limits |
| `dig` times out | Port 53 UDP/TCP open in the firewall; the service is running |
| DNS works but sites do not | Ports 80/443 must be reachable for the relay |
| Subscriber shows "not registered" | Their current public IP differs; register it with the secret |
| Phone cannot use Private DNS | Use the hostname, not an IP; it needs a valid DoT certificate |
| Locked out of the panel | Run `digitdns` over SSH → **Emergency admin reset** |
| `master.key` lost | The database is unrecoverable; restore a backup |
| Agent card says offline | Check its heartbeat: outbound HTTPS to the hub, the key, and the hub URL including the admin path |
| Agent says empty host allowlist | No active proxied domain or service resolves to it; assign one and wait for the 60 s catalogue sync |
| Agent test connection fails | The advertised relay port is not reachable: open TCP 80/443 and check the agent is binding `:80,:443` |
| Agent card cannot copy the key | The key predates encrypted storage; use **Rotate key**, then copy the new one |

### Uninstall

```bash
sudo /opt/digitdns/scripts/uninstall.sh
```

The uninstaller archives data and offers a restore script. The TUI option
**Uninstall DigitDNS** does the same.

---

## Security

- Save the generated admin password; enable **two-factor authentication**.
- Keep the hidden admin path and panel port private.
- Keep **whitelist mode** on. An open resolver is abused within hours.
- Back up `master.key` offline; it is the only way to read `data.db`.
- Expose only the ports you use. The dashboard does not need to be public if
  you reach it through a VPN or SSH tunnel.
- The API binds to `127.0.0.1` by default; widen it deliberately.
- Release checksums are signed, Docker images are cosign-signed and the
  in-panel updater verifies signatures before installing.

Full threat model and hardening guide: [docs/SECURITY.md](docs/SECURITY.md).

---

## API & automation

REST API v2 and v3 cover clients, domains, settings, resolution policy,
upstreams, diagnostics and updates. Authenticate with a dashboard session or the
master API key (`X-API-Key`). Contracts, payloads and Python/Node/cURL examples
are in [docs/API.md](docs/API.md). Swagger is served from the panel at
`/ADMIN_PATH/api/v3/docs`.

---

## FAQ

**Do I need a domain?** For the panel and client DoT/DoH, yes — the panel is
HTTPS-only and DoT/DoH need a trusted certificate. A pure DNS lab can run
without one, but that is not the supported install path.

**Does it hide my IP from websites?** Proxied domains see your server's IP, not
the client's. Direct domains see the client's own IP as usual. This is not a
VPN.

**Can I share it with family or customers?** Yes — that is what clients,
quotas, expiry, device limits and the portal are for. Keep whitelist mode on.

**Does it work behind Cloudflare or another CDN?** No. DNS and the relay need
direct access to the server's ports; a proxying CDN breaks both.

**Are Windows/macOS server binaries published?** No. The daemon builds from
source on Go-supported systems, but release binaries, installers and the
systemd unit target Linux.

**Is the Android app on Play Store?** No. Signed APKs are attached to each
release and side-loaded.

**Will upgrades keep my data?** Yes; the schema is migrated automatically. Back
up `data.db` and `master.key` first.

**Where is my dashboard layout stored?** In the encrypted database as a custom
policy, not in the browser. It is shared by every browser that signs in to the
same install.

---

## Documentation

| Document | Contents |
| :--- | :--- |
| [Tutorial](docs/TUTORIAL.md) | First boot, domain, first client, registration — start here |
| [Security](docs/SECURITY.md) | Threat model, encryption, hardening, RFC notes |
| [API](docs/API.md) | REST v2/v3 with examples |
| [Changelog](docs/CHANGELOG.md) | Every release |
| [Relay agents](docs/AGENT.md) | Install, operate and troubleshoot a geo agent |
| [Releasing](docs/RELEASING.md) | Versioning and update channels |

Release channels: `latest` and untagged `vX.Y.Z` releases are stable; `-beta`
tags are prereleases and never take the *Latest* badge.

---

## Support

If DigitDNS helps you, you can support development:

| Method | Address |
| :--- | :--- |
| USDT (TRC20) | `TWrapnstisV3ZZsEgrfKDs2552SMc86VLE` |
| USDT (ERC20) | `0xd52Edf3767080cfE3E7037b9bAd2f9Aa7a8833C4` |
| TON | `UQBV9O4TlKdnrqVqsbPD83ClBE_POBHHXD-HFYqWnqhvWDo` |

---

## License

AGPL-3.0, the same as the upstream project. This is a modified fork of
**HyperDNS** by HyperRAIN ([github.com/IzumiRain/HyperDNS](https://github.com/IzumiRain/HyperDNS));
original copyright notices are retained. See [NOTICE](NOTICE) for attribution
and a summary of changes.

Source: [github.com/digitrob/DigitDNS](https://github.com/digitrob/DigitDNS)
