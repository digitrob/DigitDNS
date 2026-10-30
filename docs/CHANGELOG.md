# 📜 DigitDNS Changelog

All notable changes to **DigitDNS** — a fork of the **HyperDNS** project by HyperRAIN — will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## 🏷️ [v3.5.3] — Wildcards you can see, and tighter rows

The wildcard scope is now a first-class control on both clients: pick exact,
*.host or *.parent, see the pattern it writes, and the rule changes in place
with its list position intact.

### ✨ Added
- **Wildcard scope menu on web and Android.** The routing menu offers **exact** (the host and every subdomain), **\*.host** (subdomains only) and **\*.parent** (the parent's subtree), with a live `applies to …` preview. Selecting a scope rewrites an existing rule immediately — no separate action needed — and `*.parent → exact` restores the original host.
- **The wildcard status under every domain.** Domains rows carry a second line: `exact`, `*.host` or `*.parent` (both when two wildcards apply), so the active scope is visible without opening a menu.
- **Protocol colours in the live log.** UDP blue, TCP sky, DoT emerald, DoH purple, DoQ amber, DoH3 orange, DNSCrypt fuchsia. The web tooltip still separates the resolution transport from the client transport.

### 🔄 Changed
- **Exact is labelled for what it always did.** A plain rule covers the host and every subdomain, so the **host + \*** option is gone; `*.host` stays for subdomains-only, and a redundant `*.domain` row beside its plain rule is hidden.
- **A wildcard change keeps the row where it is.** Patterns are replaced in the same slot, the original added-at stamp survives a rename, and A–Z sorts wildcards by their base name.
- **A scope change replaces the old scope.** The row's patterns are cleared from every list before the new ones are written, so selecting parent no longer leaves a host wildcard behind. The menu remembers what its last click wrote, so conversions inside a session always land.
- **Android Domains rows are as compact as the live-log rows** (58 dp), and the gear ripple is centred (36 dp hit area, 24 dp glyph) with the same 6 dp gap to the status pill.
- **The web quick-action menu is viewport-sized and wraps its text**, so it no longer stretches to 545 px and off a phone screen; the scope row shows three options.
- **Web Domains actions** gained a 12 px gap. The Upstream Resolvers card dropped the redundant “Racing Policy / Validated” footer, giving the resolver list its space.
- **Android live-log protocol** is coloured inline; the rest of the details line stays muted.

### 🔴 Removed
- The `host + *` scope, redundant wildcard rows left by older pairs, and the menu's `saved:` / “not saved yet” status line.

### 🐛 Fixed
- **Scope buttons could silently do nothing.** After `*.parent` the origin no longer existed in the lists, so the next scope click moved only the pill; conversions now retarget through the pattern the menu last wrote.
- **The popup and the stored rule could disagree** about the active wildcard; both now come from the same pattern.
- **The live log's PROTO colour matches the transport actually shown** — the resolution protocol, falling back to the client transport.

## 🏷️ [v3.5.2] — Strategies you steer, and one Domains list

The resolution policy gained two strategies that trade queries for latency, the
web Domains page learned that a managed domain is not always a proxied one, and
the live log began naming the transport that resolved the query.

### ✨ Added
- **Hedged and Best Known resolution strategies.** Both are selectable globally and per domain. **Hedged** starts the fastest measured resolver and adds the next one every 50 ms, cancelling the losers as soon as a usable answer arrives — a fast first resolver still sends exactly one query. **Best Known** uses the fastest measured resolver only and falls through the rest in latency order on failure. Both rank from the live racer stats rather than the configured order.
- **One Domains list on the web dashboard.** Proxied, paused, blocked and direct names share one table. **Proxy / Blocked / Paused / Direct** chips filter it with no separate All chip (tapping the active chip clears the filter), a status change keeps the row in place, and the status badge opens the routing action menu.
- **The query log carries the resolution transport.** `upstream_protocol` (UDP, TCP, DoT, DoH, DoQ, DoH3, DNSCrypt) joins the payload alongside `matched_pattern`; the web PROTO column and the Android row show how the *resolver* was queried, and the web tooltip keeps the client's own protocol.
- **Android's action badges speak English.** `Proxy IPv6 Sink`, `Rate Limited`, `Proxy Failed`, `Stale` and friends replace `Proxy_ipv6_sink`-style labels through one shared table, and an action this build has never seen is word-split instead of printed raw.

### 🔄 Changed
- **Every rule falls back to the pool now.** The Failure (SERVFAIL) choice is retired; stored `fail` profiles are read as pool fallback and rewritten on the next save.
- **Blocked and direct rows keep their controls, disabled.** The resolution gear and pause switch stay visible but dimmed on both clients, while paused rows keep both usable. Changing a status never reflows the row.
- **Wildcard scopes say what they will write.** The domain action menus label the choices `exact`, `*.host` and `*.parent`, disable the impossible ones before a click, and `*.host` means subdomains of the host only — not the host itself.
- **Row geometry is stable.** Android measures the status pill once from its longest label, so PROXY / PAUSED / BLOCKED / DIRECT never shift the gear, with 6 dp on either side of the pill. The web resolution card puts each label 8 px from its dropdown and lets the dropdown fill the row instead of a fixed 208 px; Android's resolution pickers use a fixed 200 dp button and a 240 dp menu.

### 🔴 Removed
- **The wildcard-match chip in the live log** (`*.PARENT` / `*.HOST` / `WILD`) on web and Android. `matched_pattern` stays in the API for tooling.

### 🐛 Fixed
- **Android's live-log action filters missed outcomes.** Proxy now matches `PROXY`, `PROXY_IPV6_SINK` and `PROXY_FAILED`; Direct matches `DIRECT`, `CACHED` and `STALE`; Block matches `BLOCK`, `RATELIMIT` and `QUOTA`.
- **A mixed selection could try to pause blocked rows.** Bulk pause/resume now applies only to proxied and paused rows; Remove still handles the rest.
- **The live log can no longer disagree with the resolver that answered.** PROTO comes from the winning upstream's address, and only cache, local, blocked and proxied answers fall back to the client transport.

## 🏷️ [v3.5.1] — Menus that belong, and resolvers that lead with the transport

A small UI follow-up to 3.5.0.

### 🔄 Changed
- **Android's dropdown menus look like the app.** The Transports, Default upstream and Strategy pickers on the Upstream Resolvers card, the four pickers in **Override Resolving**, and the Domains sort menu now use the same 12 dp card surface, cyan hairline and glass fill as the panels instead of Material's default menu background.
- **Menus open under the button that opened them.** The Domains sort options used to appear beside the search row; the menu is now anchored to its selector. The resolution pickers get the same anchoring.
- **A selected transport leads the resolver list.** With **Encrypted Only**, encrypted resolvers move to the top; with **Regular DNS**, plain ones do. The server's latency order is kept inside each group. **Any Transport** leaves the order untouched.

## 🏷️ [v3.5.0] — Resolution you can see and shape

The upstream pool became a policy you can reason about: one global default, a
transport that is actually enforced, and a per-domain override where the domain
lives. The resolver card also shows what that policy is doing.

### ✨ Added
- **Per-domain resolution overrides on the Proxied Domains list.** The gear on a proxied or paused row opens **Override Resolving**: choose a resolver, a racing strategy, the transport and what happens when it fails. Rules belong only to names in the proxied lists, and removing a domain removes its rule.
- **Transports are a real choice.** **Any Transport**, **Encrypted Only** (DoT · DoH · DoQ) and **Regular DNS** (UDP · TCP), globally and per domain. Encrypted Only is enforced: a pinned plain resolver is ignored while it is active, and the dialog warns instead of pretending the pin works. The encrypted option is offered only when an encrypted resolver is configured.
- **Global defaults in the Upstream Resolvers card.** **Default upstream** (`Pool Default` or a specific resolver), **Strategy** (Fastest Wins / In Order) and **Transports**, applied to every name without an override.
- **Failure is explicit.** A rule can **Fall Back to the Pool** or answer **Failure** (SERVFAIL) rather than silently changing transport.
- **The live query log names the upstream that answered.** Both the web panel and the Android app show `via …` under the domain, so a resolution rule can be checked against what actually happened.
- **`/api/v3/settings/resolution`** accepts a dashboard session *or* the master API key, and the v3 upstream add/remove/reorder calls now persist the resolver list into settings.

### 🔄 Changed
- **Upstream Racers is now Upstream Resolvers**, on both clients.
- **The resolver list shows five rows and scrolls by itself.** The add row, resolution defaults and footer stay fixed; the card no longer grows with the resolver count.
- **Rows dim to show the policy.** A chosen default resolver fades the others; Encrypted Only fades plain resolvers; Regular DNS fades encrypted ones. The list remains fully interactive.
- **Override dialogs are compact and opaque.** The page stays visible behind them (no blackout, no window dim on Android), the title is **Override Resolving**, and the failure choice reads **Failure**.
- **Upstream and transport stay in sync in the override dialog.** Picking a resolver selects its transport; picking Encrypted Only or Regular DNS selects a resolver of that type.
- **Smaller UI moves.** The Android Domains gear is larger and sits clear of the status pill; the web gear is a real gear, placed before the Pause switch; the default-upstream placeholder reads **Pool Default**.

### 🐛 Fixed
- **API-key clients could not load or save resolution settings.** The route lived on the session-only panel surface, so an app signed in with the master key got `401` while every other v3 call worked. It now lives on the v3 router and accepts both credential types.
- **Resolvers added through the API disappeared from the settings view.** The v3 add/delete/reorder handlers only changed the running pool; the resolver card and the encrypted-transport gate kept the old list. They now persist before the pool changes.
- **Encrypted Only and Default upstream looked unavailable or stale.** Both are derived from the live pool as well as the saved settings, and the card refreshes after a resolver changes, so a newly added DoT/DoH/DoQ resolver enables the choice immediately.
- **The override dialog could open with only “Inherit Global” in the upstream picker.** It now reloads on open, falls back to the live pool if the settings request fails, and shows a retry instead of a silently shorter list.
- **Legacy settings migrate cleanly.** An old `encrypted_only` boolean is read as the equivalent transport and rewritten on the next save.

## 🏷️ [v3.4.11] — Tighter lists and a log that fits

### 🔄 Changed
- **The live log's ACTION column fits `PROXY · IPv6 SINK` on one line.** It grew from 8rem to 9.5rem; the domain column is the flexible one, so it gives up exactly the same 24 px. Action chips no longer break across lines, and the table now has a minimum width, so a narrow panel scrolls sideways instead of collapsing the domain column to zero.
- **Android's Domains and Live log rows are compact and the same height.** Both lists use a shared 56 dp row: 8 dp top/bottom padding instead of 16, and the Domains list keeps 8 dp between boxes. They read at the same density and line up page to page.
- **The Android live log no longer shows a third line.** The rule-matched text (`Self-service`, `Direct`, `Cached stale`, …) is gone; domain and client/protocol/latency/time remain, with the action badge.

## 🏷️ [v3.4.10] — Small fixes for upstreams and the dashboard

### ✨ Added
- **Long upstream addresses are readable in the dashboard.** The name in the Upstream Racers list is now a scrollable field rather than an ellipsis: drag it, swipe it, or focus it and use the arrow keys to read the whole DoH URL. A soft fade marks the edge the text can still travel towards, and the latency badge and delete button never move. The cursor stays the ordinary arrow.

### 🐛 Fixed
- **A DoH upstream with the port written after the path is rejected instead of stored.** `https://dns.adguard-dns.com/dns-query:443` looked like a valid entry, but the port became part of the path, so every request went to `/dns-query:443` and got a 404 — visible only as intermittent `[DNS] Upstream query error … returned HTTP 404` lines. The add handler now refuses that shape and the error says where the port belongs (`host:443/dns-query`).
- **The login screen no longer flashes `v2.1.0`.** The footer's pre-hydration fallback was the last stale version string in the panel; it now shows a dash until the real version arrives.

## 🏷️ [v3.4.9] — Bug fixes and small improvements

### 🔴 Removed
- **The v1 REST API is gone.** Its published sunset was 1 July 2026, and it outlived it by three months. `/api/v1/*` now answers 404: version, status, clients, policies, cache flush, API-key rotation and the v1 docs page. Every operation is served by `/api/v2` (or `/api/v3` for dashboard parity), and the panel's copy-paste examples, the docs and the bundled Swagger viewer all point there. `/api/server/restart` is unversioned and stays. Container health checks moved to `/api/v2/version`.
- **Dead code.** The pre-`DigitDNS` interactive console (`internal/tui/manager.go`, `webport.go`, `unlock.go` and their tests — `tui.Run` is the live entry point), the unused link-preview detector, three unreferenced helpers, the retired quick-action CSS (sixteen brand accent rules and the button styles), and the stale `v2.1.0` version fallback in the panel HTML.

### ✨ Added
- **A test that stops the offline bundle's version drifting.** `offline-bundle/version.json` had been left at 3.0.0 while the daemon shipped 3.4.7; the only signal was a startup warning. The build now fails if the two files disagree.

### 🔄 Changed
- **The live log's domain column keeps a readable width.** The six fixed columns added up to 50.5rem, so on a 1280-wide laptop the domain column received ~30px and every query read as `n...` with the headers overlapping. Narrower fixed columns give the domain a usable share; the table still scrolls horizontally on phones.
- **Secondary labels and numbers use one convention.** RAM Cache says `% hits` everywhere, and the chart's tick labels use a dot as the decimal separator instead of the browser locale's comma.
- **Browser storage uses the current name.** `hyperdns_token` and `hyperdns_theme` are read once for continuity and then replaced by `digitdns_*`.
- **`405 Method Not Allowed` from v2/v3 now carries an `Allow` header** derived from the handler's own verb list, as RFC 9110 requires.
- `.opencode/` is ignored by git.

### 🐛 Fixed
- **A fresh install with `access.allow_all: true` in config.json now keeps it.** The v2.2.0 whitelist migration treated a brand-new database like an upgrade and forced `allow_all=false`, silently overriding the file and refusing every query until an operator flipped the switch. A first boot with an explicit file value now writes the migration marker without touching the value.

## 🏷️ [v3.4.8] — A live log that really clears, and tighter tiles

### ✨ Added
- **Clear now clears the server, not just the tab.** The live log's Clear button empties the daemon's in-memory ring through a new `POST /api/stream/clear`, and every open dashboard drops its rows on the resulting `cleared` event. Before, only the browser buffer was emptied, so the next reconnect replayed the history the operator had just dismissed.
- **The Android server box reads your server, not just its address.** Swipe the value sideways to blur the IP or domain for privacy; swipe it up or down to swap between the public IP and the domain the install answers on. The gesture fires as soon as it crosses the threshold, works in either direction, and lives on the text only — the flag and copy icon are never touched.
- **Zebra rows in both dashboard lists.** The Proxied Domains table and the live query log alternate a faint row tint, so a dense wall of monospace no longer forces the eye to re-find the line it was reading. Paused proxied domains get an amber wash instead of the old `opacity-50`, which used to fade the switch and the status badge along with the domain.

### 🔄 Changed
- **The Android dashboard tiles are compact.** All six metric cards (Query Rate, RAM Cache, SNI Proxy, CPU, RAM Usage, Live Traffic) share a 22 sp number (was 32 sp), an 11 sp supporting line, 8 dp inner padding and a 94 dp minimum height (was 166 dp) — nearly half the height it shipped with — while every label stays on one line, so a row of two stays level and more of the grid fits on screen. Live Traffic's `Total` caption now sits on the same line as the ↓/↑ rates, RAM Cache's `% hits` sits under its number, the SNI footer uses the same secondary colour as every other card, and every card icon is 14 dp.
- **The server-switch button matches the address box.** It is a rounded square the height of the row, shortened to sit flush with the box's border, sits the same 12 dp away as the grid's own card gap, and its icon is centred.
- **Upstream Racers has no rank numbers.** The 1 2 3 4 5 column is gone; each row starts with its status dot, address and hit/error count.
- **The live log holds a hundred queries, on both sides.** The browser buffer now matches the daemon's ring instead of keeping twice as many rows, and a second tab or a reconnect never shows more than the server would replay.

### 🐛 Fixed
- **The live log no longer refills after Clear, repaints while paused, or loses arrivals during a pause.** The `history` listener ignored the pause switch and repainted on every reconnect (which is what made Clear look broken); a coalesced render frame could re-insert arrivals queued before a Clear; and pausing dropped arriving queries instead of buffering them, so Resume showed a gap. All three are fixed, and the panel says so when the server-side clear fails.
- **The container no longer reports itself unhealthy when the panel runs over HTTPS.** The image's health probe asked `http://127.0.0.1:<web_port>/api/v1/version`, but a `panel_https` install serves TLS on that port only, so every probe was answered with a handshake error and Docker marked a perfectly working container as sick. The probe now tries HTTPS first (skipping verification, because it is a loopback liveness check against the operator's own certificate) and still falls back to HTTP for installs without `panel_https`.

---

## 🏷️ [v3.4.7] — See where your server is, and update it from the app

### ✨ Added
- **A country flag beside the server IP.** The web panel's IP box and the Android dashboard box now show the flat 4:3 flag of the country the server's public address resolves to. Flags are bundled with each client, so they look the same everywhere and no external image host is contacted.
- **A GeoIP database that keeps itself current.** The daemon keeps a GeoLite2-format database in its data directory and refreshes it once a day in the background. Lookups are local — client addresses never leave the box. If a download fails, the last good database keeps working.
- **“Update database” on demand.** The Server Location card on the web (Settings & SSL) and on Android (About) shows the country, the database date, and a button that fetches a fresh copy immediately.
- **Server Update on the web.** Settings & SSL now carries an update card beside Server Location: it checks for a new release and, when one is available, runs the same verified updater the sidebar badge uses — download, checksum verification, backup, restart and progress. The two cards share the row, half width each.
- **Server Update on Android.** The About page's update card now mirrors the Server Location card — a refresh icon on “Check for updates”, aligned buttons, and a clear “not checked / up to date / update available” line.

### 🔄 Changed
- **Country flag placement.** The flag is centred against the whole server-IP block (heading, address and copy icon), not pinned to the address line, and sits a few pixels clear of the copy icon.
- **GeoIP dates read DD.MM.YYYY** everywhere, instead of an ISO date.
- **The Update box is called “Server Update”** on both clients.
- **Android About is tighter** — 8 dp card spacing and the restructured update card keep the page to one screen on a normal phone.
- Server and client expose the country through the existing settings/config API plus two small admin endpoints, so future features (client countries, per-country stats) can reuse the same lookup.

### 🐛 Fixed
- The small “Updates come from the GitHub release channel” line is limited to two lines, so the new icon cannot push it into the button.

---

## 🏷️ [v3.4.6] — Certificate controls in one place, and a faster proxied list

### ✨ Added
- **Certificate renewal is now on the SSL/TLS page.** "Certificate Auto-renew" sits between "Issue a certificate" and "DoT/DoH Auto-renew", with the line *Renew Let's Encrypt SSL certificate.* It is the same switch that used to live on the Subscription page — it is only the home that moved, so nothing about how the certificate renews has changed.
- **Instant search on the Proxied Domains page (web).** The sort menu, the domain count, a new search box and the Pause/Resume/Remove buttons now share one row. Typing narrows the list on every keystroke, exactly like the Live Query Log, and the count tells you how many matches there are.
- **Autocomplete when adding a domain (web).** The Add field suggests names that are already listed, and the table narrows as you type, so a duplicate is visible before you press Add. Pressing Add on one anyway now says "… is already listed" instead of quietly doing nothing.

### 🔄 Changed
- **Both renewal switches now tell the truth.** The DoT/DoH switch used to start "on" without asking the server; it now reads the stored setting. The new certificate switch does the same.
- **"DoT/DoH Auto-renew"** is capitalised to match the other card titles.
- **The server switch on the dashboard** sits in the server-IP row, lined up under the ONLINE badge instead of floating below it.
- **Select-all on Proxied Domains** now selects only the rows the filter is showing. A selection survives filtering and clearing the box, so you can search in batches and act on all of them.
- **The SSL/TLS page** uses slightly tighter spacing so the extra card still keeps the whole form on one screen.
- **The Subscription page** keeps a single "Subscription portal" switch.

### 🐛 Fixed
- **Overlapping text on the Subscription page.** The two switches were being drawn on top of each other in the same card, which produced the garbled "Auto-renew certificate / Subscription portal" text. Moving the certificate switch to SSL/TLS leaves one clean row, and the second switch is reachable again.
- **A stray suggestion dropdown on the proxied search box.** It now behaves exactly like the live-log search: type and the list narrows, with no browser popup in the way.

---

## 🏷️ [v3.4.5] — Dashboard, server management, and client editor polish

### ✨ Added
- **Manage servers, rebuilt for a phone.** The server manager now fits on one fixed screen: up to three saved servers are visible at once, and only that list scrolls when you have more. Add server, the signed-in details and Sign out sit at the bottom, next to Cancel/Done, so nothing is pushed off-screen.
- **Cancel on the Add-server screen.** Opening "Add server" is no longer a one-way trip — Cancel takes you straight back to the app and throws away a half-entered host or credential.
- **Reset layout for dashboard cards.** App → Settings → Dashboard cards now has a Reset layout action that brings back the built-in order and visibility.
- **Duplicate domains are ignored.** Rule lists are de-duplicated when loaded and when saved, so a repeated entry can no longer clutter a list — or crash the app.
- **Version line in the startup banner**, so the running build is visible in `journalctl` / `docker logs` without a separate request.

### 🔄 Changed
- **Dashboard default order.** Upstream Racers comes first, followed by the six metric tiles in two columns (Query Rate, RAM Cache, SNI Proxy, CPU Usage, RAM Usage, Live Traffic), then the full-width Live QPS & Telemetry chart, then Resolver Latency. The six metric tiles now share one fixed size, so the grid no longer looks ragged.
- **Domains rows (Android).** Each row is one clean line again: the domain on the left, the status pill and power switch on the right. Nothing shifts, and the row height stays constant.
- **Proxied Domains table (web).** Domain names take the available width and stay left-aligned; the per-row switch/actions and status become compact columns on the right. Space is used rather than spread evenly.
- **Client editor (web).** The expiry field is simply "Expiry" and takes less room; "IP Addresses" is a large multi-line box with the space that frees up. Saving writes every field, closes the card, and tells you when the device limit kept fewer addresses than you entered.
- **Client cards (web).** A subscriber with many registered addresses now shows them in a scrollable window, so the card keeps the same size as one with a couple of IPs.

### 🐛 Fixed
- **Domains tab crash.** Opening Domains against a server whose proxied list contained the same domain twice crashed the Android app. Duplicates are now ignored on both ends; the tab opens normally.
- **Accidental upstream deletion.** Scrolling the dashboard could arm the Upstream Racers swipe and pop "Remove upstream?". Scrolling now disables the gesture, and reaching the confirmation needs a near-full-width swipe.
- **Failed sign-in shown as a session.** A saved server you tapped but never authenticated to no longer appears in "Signed in to"; it stays in the saved list, where it belongs.
- **Saving a client did nothing (web).** The save path is now bound to both the form and the button, survives a partial page initialisation, reports the server's error instead of failing silently, and parses expiry dates correctly on Safari.
- **Date Added stamps removed** from the web and Android proxied-domain lists. The server still records the times internally for future use; nothing is displayed.

---

## 🏷️ [v3.4.0] — Android & web parity

### ✨ Added
- **API 3.1 client fields.** `GET /api/v3/clients` and its item routes now return `total_queries`, `last_seen` and `next_traffic_reset` — the figures the web card has always read off the legacy shape. All three are additive; a v3.0 caller keeps parsing the response unchanged.
- **Editable UUID and absolute expiry over v3.** `POST /api/v3/clients` accepts `expires_at` (which wins over `validity_days`), and `PATCH/PUT /api/v3/clients/{id}` accepts `uuid` and `expires_at`. The zero timestamp `0001-01-01T00:00:00Z` clears an expiry; omitting the field leaves it untouched.
- **`api_version` in `GET /api/v3/version`.** Reports the contract generation (`3.1`) next to the existing `api: "v3"`, so a client can discover the additive fields without probing. The URL prefix is unchanged.

---

## 🏷️ [v3.3.2] — Banner actually reaches `docker logs`

### 🐛 Fixed
- **The new wordmark's tail never reached the container log.** The daemon printed its banner with a final line that carried no trailing newline. The console doesn't care, but Docker's log reader emits a piped line only at a newline: `docker logs` and Portainer showed the banner minus its last line (the fork attribution) for as long as the container ran; it only leaked out at exit. The startup write is now newline-terminated, with a regression test pinning the shape.

---

## 🏷️ [v3.3.1] — Branded banner and reachable addresses in the startup summary

### ✨ Added
- **DigitDNS wordmark.** The daemon startup log and the interactive console now print the DigitDNS art the codebase already carried, ending the three-way duplication between the daemon banner, the console header and the installer. The banner carries the fork attribution (`fork of HyperDNS by HyperRAIN · AGPL-3.0 · digitrob/DigitDNS`), so the project's provenance survives anywhere an operator reads the banner — journald, Portainer, docker logs.

### 🔄 Changed
- **Startup summary shows reachable addresses.** The boot footer used to print its listeners against the bind wildcard (`https://0.0.0.0:27586/...`), which no browser, device or subscriber config can open. Each line now resolves the host with a fixed preference — panel domain, then public IP, then the bind host as a last resort for loopback/SSH-tunnel deployments. A dedicated DoT domain outranks the panel domain on the DoH and DoT lines (the only name their certificate is guaranteed to cover), mirroring the dashboard's subscription-origin logic; a default DoH port (443) is suppressed from the URL the same way.

---

## 🏷️ [v3.3.0] — Android companion app, dialog UX, and live-log polish

### ✨ Added
- **Android companion app.** A native Android client (minSdk 29) with full panel parity: login with session tokens and TOTP, dashboard stats and QPS chart, domain rules, clients, upstreams, live query log, diagnostics and settings. It runs its own foreground stream service with notifications, ships as signed multi-ABI APKs (`digitdns-<abi>.apk`), and carries its own version line.
- **Client management as popups.** Adding and editing a client now opens over the list instead of pushing a new screen, so the operator keeps their place; the form is shared between the two so they cannot drift.
- **Adaptive launcher icon.** The Android app now uses a proper adaptive icon that survives every OEM mask (circle, squircle, Samsung rounded square) without clipping the emblem.
- **Swipe-to-delete with confirmation.** List rows delete on a swipe, but only after an explicit confirmation dialog; a cancelled swipe snaps the row back. The old four-second undo toast is gone in favour of the confirmation.
- **Dedicated pull-to-refresh indicators.** Servers, TLS, About, Groups, Policies and the Live Log now raise a real spinner for the duration of a refresh instead of reusing a flag that only meant "busy".

### 🔄 Changed
- **Live Query Log.** Reload (no spinner) and pull-to-refresh (spinner) are now distinct, mirroring the Domains page; the toolbar buttons keep fixed widths so toggling Pause/Resume can no longer shift Clear off the edge; buttons wrap instead of collapsing on narrow phones.
- **Client detail popup.** Opens without a flash of the previous client or a loading spinner showing through the dialog; the form renders as an offline form.
- **Dashboard stat cards.** Totals and secondary figures are right-aligned / centred so nothing is truncated at narrow widths.
- **TLS issuance.** A forced renewal now skips the on-disk fast path and contacts Let's Encrypt, for compromised keys or mis-issued certs.
- **Upstreams API.** New `GET /api/v3/upstreams/stats` and `POST /api/v3/upstreams/delete`, matching the dashboard.
- **Panel transport.** The panel now negotiates HTTP/1.1 only, so long-lived live-log streams are not reset by HTTP/2 intermediaries.
- **Browser forms.** Password fields are wrapped in no-op forms with a hidden username, satisfying Chrome's autofill/accessibility checks without changing masking.

### 🐛 Fixed
- Swipe deletes no longer happen outright; they always ask first.
- Client detail popup no longer flashes stale state or closes on a leftover `saved` flag.
- Live Log now re-anchors on both Reload and pull-to-refresh.
- Toolbar buttons no longer collapse or clip their labels.


## 🏷️ [v3.2.2] — Docs links, gated-docs 403 page, leaner container

### 🐛 Fixed
- **The v2 and v3 API Reference links in the dashboard banner no longer 404.** The banner's root-absolute `/api/v2/docs` and `/api/v3/docs` hrefs were never rewritten to the install's hidden admin namespace — the server rewrote only the v1 anchor, and `app.js` rewrote only v1 at runtime — so both opened the host root, where the API is not mounted. The server-side document rewrite and the runtime rewrite now cover all three versions; each button opens the page the server actually serves.
- **CPU telemetry now works on macOS and the BSDs.** The Unix CPU sampler was tagged `!windows` but read `/proc/self/stat`, a Linux-only file — so on a macOS or BSD host `processCPUSeconds` errored on every tick and the dashboard's CPU tile never reported a figure. The sampler is now split by platform: Linux keeps `/proc/self/stat`, and Darwin/the BSDs read the same cumulative user+system time through `getrusage`.
- **A browser refused by the API bind gate on a docs page now gets an explanation instead of raw JSON.** When `api_bind` (or `api_bind_list`) does not admit the caller, `/api/v?/docs` and `/api/v?/openapi.json` answer with a self-contained HTML 403 that names the rule in force and lists the three ways to reach the docs — add the address in **Settings → API Network Binding**, connect over an already-allowed network (for example a tailnet), or open an SSH tunnel and browse over loopback. The API data routes keep their JSON 403, because their callers parse a body rather than read a page.

### 🔄 Changed
- **The container image builds in fewer layers and re-uses its caches.** The runtime `apk add` and directory creation are one layer, the Go module download and compile use BuildKit cache mounts, and the binary is built with `-buildvcs=false` alongside `-trimpath` for reproducible bytes. The base images are unchanged: `golang:1.26-alpine` to build, `alpine:3.20` to run.

### 📝 Notes
- API documentation (including the OpenAPI specs) **remains gated** by the API network binding; it is not exposed to callers the rule does not admit. Only the refused response changed.

---

## 🏷️ [v3.2.1] — API v3: correct problem `instance`

### 🐛 Fixed
- **RFC 9457 `instance` now names the route the error came from.** Every API error body carries an `instance` member, but it was a fixed `/api/v2` string for all versions — so a `/api/v3/…` failure reported itself as a v2 problem. v3 problems now point at the actual request path (e.g. `/api/v3/clients`); v2 responses keep their `/api/v2` prefix unchanged.

---

## 🏷️ [v3.2.0] — API v3: Full Dashboard Parity with Session Login

### ✨ Added
- **REST API v3 (`/api/v3`) — full dashboard parity, in one release.** Everything the web panel can do is now reachable without a browser: clients, policies, custom groups, rules, upstreams, settings, subscription, TLS/ACME issuance and renewal, diagnostics targets and runs, the live statistics stream, cache flush, self-update and the raw config. v3 keeps every v2 convention a caller already knows — RFC 9457 `application/problem+json` errors, `{items, next_cursor}` pagination with a bounded `limit`, symmetric DTO field names and `POST /{resource}/{id}/actions/{action}` sub-resource verbs — so a v2 client is a v3 client with more routes.
- **Session login for the API.** v1 and v2 accept only the master REST key; the dashboard accepts only a dashboard session. v3 accepts **both**, so a companion app (the forthcoming Android client among them) can log in with the operator's username and password — plus a TOTP code when two-factor is on — and hold a real, revocable `digitdns_session_` token, while existing scripts keep using the key unchanged. `POST /api/v3/auth/login` runs the same KDF, lockout tracker and replay guard the panel login uses; `/auth/logout` revokes, `/auth/me` reports state, and `/auth/unlock` (master-key only) is the lockout lifeline.
- **One-time stream tickets for SSE.** `POST /api/v3/auth/sse-ticket` mints a single-use ticket so an `EventSource`, which cannot send headers, can still reach `/api/v3/events/stream` without putting a long-lived credential in the URL.
- **2FA enrolment over the API.** `/api/v3/auth/2fa/{status,setup,enable,disable}` mirror the dashboard's own enrolment endpoints.
- **OpenAPI 3.0.3 spec and docs.** `/api/v3/openapi.json` and `/api/v3/docs` describe the whole surface; the Developer API Gateway banner now leads with **API Reference v3**, with v2 and the v1 (deprecated) docs one click away.

### 🔄 Changed
- The dashboard's **Developer REST API** banner is retitled **Developer REST API v3** and links to the v3 reference, with compact secondary links to the v2 reference and to v1 flagged *deprecated, sunset Jul 2026*.
- The dashboard-coupled v3 routes delegate to the web server's own handlers through an injected backend interface, so the REST answer can never drift from the button it mirrors: one implementation per feature.

### ⚠️ Deprecated
- **REST API v1** now advertises `Deprecation: true` and `Sunset: Wed, 01 Jul 2026`. It continues to work unchanged; new integrations should target v3.

---

## 🏷️ [v3.1.1] — Diagnostic Suite: stale result on server removal

### 🐛 Fixed
- **Deleting a server in the Diagnostic Suite now clears its test result immediately.** Removing a target from **MY SERVERS** updated the editable list and reset the score banner, but the run-results row for that server stayed on screen until the next **Run Test** rebuilt the whole list — so a removed server appeared to still be measured. The results row now carries the host it measured and is dropped the moment its target is deleted, with the empty-state hint restored when no rows remain.

---

## 🏷️ [v3.1.0] — Multi-CIDR Resolver & API Access, Portal IP Self-Service & a Rebuilt Diagnostic Suite

### ✨ Added
- **Multi-CIDR resolver access.** The allow/deny gate now accepts full CIDR blocks as well as single addresses, so an operator can admit a whole subnet without listing every host. Server-wide CIDR entries are edited in the whitelist card (whitelist mode only) and are logged and metered as the anonymous `Public` account — no quota, no expiry, never counted against a subscriber's `MaxDevices`. Subscriber-registered exact IPs continue to count toward device limits; operator CIDRs do not.
- **Subscriber portal IP self-service.** A registered subscriber can see the list of IPs currently bound to their account and register an additional IP through a dedicated "Update my IP" flow. The register secret remains mandatory, so an address is only ever bound by someone who holds its credential.
- **Multi-CIDR REST API access.** The API bind gate accepts a **list** of addresses and CIDR blocks (`api_bind_list`) in addition to the single `api_bind` rule, over both the dashboard and the v2 REST API. The union admits a peer if any rule matches; loopback is always admitted and an unparseable rule fails closed.
- **Rebuilt Server Diagnostic Suite.** The built-in game presets are gone. The suite is now wholly operator-owned: you add your own servers by name and host, and the dashboard and the v2 REST API run the same shared probe so the two surfaces can never drift. Results report per-target reachability and latency, summarised as a rounded percentage of reachable targets.

### 🔄 Changed
- **`api_bind_list` outranks `api_bind` when both are set.** A non-empty list is authoritative and the single rule is reported back as `127.0.0.1`; clearing the list falls back to the single `api_bind` rule. The precedence is resolved once, in `ServerSettings.GetAPIExposure()`.
- The dashboard exposes the resolver allowlist as a dedicated **Custom addresses / CIDRs** editor alongside the single **Custom IP / CIDR** editor, and the root `config.json` accepts an `api_bind_list` array (normalised on load).
- The API status responses now report `api_bind_list` and a resolved `api_exposure` field.
- The whitelist card copy now reads "…use the service".

### 🐳 Distribution
- Binary targets are **Linux only** — `linux/amd64` and `linux/arm64` — alongside the published Docker image; the cross-build matrix and release workflow were narrowed accordingly.

---

## 🏷️ [v3.0.0] — Full DigitDNS Rebrand, Proxied Domains UX & Forked Self-Updates

This fork is named **DigitDNS** throughout: the install tree, the systemd unit and service,
the Go module path, the CLI, the environment variables, the API-key and session prefixes and
the HTTP control headers all carry the new name (see `NOTICE` for what is deliberately kept).
Every change below is carried on the `main` branch.

> **A note on the history below.** Every section from `v2.8.0-beta.1` downward is inherited
> from upstream HyperDNS and is reproduced verbatim, so its product names, service names,
> command examples and `HYPERDNS_*` environment variables describe the upstream project as it
> was at that release — not this fork's current names. Where this fork changed a name, the
> instructions that follow from it live in [`README.md`](../README.md),
> [`TUTORIAL.md`](TUTORIAL.md) and the installer banners.

### 🏷️ Rebrand
- **The project is renamed HyperDNS → DigitDNS across the whole tree.** Concretely: the Go module is `digitdns` and the command lives at `./cmd/digitdns`; the install tree moves `/opt/hyperdns` → `/opt/digitdns`; the systemd unit and service become `digitdns.service` / `digitdns` with the resolved drop-in `digitdns.conf` and `RuntimeDirectory=digitdns` on `/run/digitdns`; the console command and its symlink become `digitdns` (the `hdns` name is retired); every environment variable is `DIGITDNS_*` (`DIGITDNS_DOMAIN`, `DIGITDNS_EMAIL`, `DIGITDNS_FRESH`, `DIGITDNS_REF`, `DIGITDNS_PUBLIC_IP`, `DIGITDNS_HEALTH_PORT`, …) with no `HYPERDNS_*` fallback; the API key stamps `digitdns_live_`, sessions use `digitdns_session_`, and the HTTP control headers are `X-DigitDNS-*`. Release assets, backups and logs follow suit (`digitdns-linux-<arch>`, `digitdns-preinstall-*`, `digitdns-backup-*`, `digitdns-before-restore-*`, `digitdns-premigration-*`, `digitdns_out.log`).
- **An existing HyperDNS install is migrated in place.** Both installers detect a legacy `/opt/hyperdns` tree, archive it to `/root/hyperdns-premigration-<ts>.tar.gz`, move it to `/opt/digitdns`, rename the old binary and the systemd unit and drop-in, and then continue the install — so `data.db`, `master.key`, `config.json` and `certs/` are preserved exactly, and the panel port, admin path and credentials survive. The safe-install wipe is skipped on such a run and no fresh credentials are generated.
- **A stored API key is rotated to the new prefix once, at daemon start.** A migrated database still holds an `hdns_live_…` key, and the dashboard gate routes on the current prefix, so `migrateAPIKeyPrefix` rewrites the marker to `digitdns_live_` (the random secret tail is untouched) the first time the upgraded binary opens the database. Legacy `hdns_live_` remains recognised only by that migration.

### ✨ Added
- **Encrypted upstreams (DoT, DoH, DoQ, DoH3 and DNSCrypt) in the Upstream Racers.** An upstream entry may now name a transport: plain `1.1.1.1` stays UDP, `tcp://host:53`, `tls://host` (DoT, default port 853), `https://host/dns-query` (DoH, RFC 8484, default path `/dns-query`), `quic://host` (DoQ, RFC 9250, default port 853, alias `doq://`), `h3://host/dns-query` (DNS-over-HTTP/3, default path `/dns-query`, alias `doh3://`) and `sdns://…` DNS stamps (DNSCrypt plus stamped DoH/DoT) are queried on their own transport and benchmarked the same way. Every encrypted transport verifies the upstream's certificate (no `InsecureSkipVerify`). The add box accepts and canonicalises any of these forms, and the dashboard shows the reason when a value is rejected.
- **Proxied Domains list controls** — a sort selector (Added newest/oldest, Name A–Z/Z–A, Status) and a batch-size selector (10/25/50/100, default 25). The list loads incrementally with an IntersectionObserver sentinel and a "Load more" fallback.
- **Live Query Log row actions** — each row's ACTION badge opens one shared menu offering Proxy / Block / Direct / Remove for the exact host, its `*.host` wildcard, or the `*.parent` wildcard. Applying one rewrites the routing lists through the exact same write path as a manual edit.
- **Bulk import for Custom Static Records** — the static-record card gains a **Bulk import** button that loads a whole `hostname address` table at once (paste or file, one pair per line, separated by spaces/commas/semicolons/`=`, IPv4 or IPv6). It reuses the shared import dialog and reports how many records were added, updated, unchanged or invalid, so re-uploading a corrected table refreshes existing addresses instead of duplicating them.
- **About & Legal card and footer notice** crediting upstream HyperDNS, stating the AGPL-3.0 licence and naming the fork's source.
- **`NOTICE`** file with the AGPL-3.0 attribution and a modification summary relative to upstream.
- **`upstream-sync` GitHub Actions workflow** — merges upstream into a throwaway branch and opens a pull request, raising an issue instead when the merge conflicts.

### 🔄 Changed
- The dashboard section formerly called **"Custom Proxied Domains"** is now **"Proxied Domains"** (sidebar, heading, mobile label and i18n string).
- The **Live Query Log** fills the available height (the centred max-width is preserved) instead of a fixed 500px scroll box.
- **Self-updates are repointed to this fork**: `internal/selfupdate` targets `digitrob/DigitDNS` on the `main` branch and pins the fork's own ed25519 release key.
- **Proxied Domains table headers** are aligned with the columns they label.
- **The interface is English-only.** The Persian locale, its RTL font stack and every translated string were removed from the dashboard and the subscriber portal, and the shipped `README.fa.md` and `docs/TUTORIAL.fa.md` translations were dropped. Vazirmatn remains only as a Latin-capable display font.

### 🗑️ Removed
- **LDAP directory login.** The `internal/auth` LDAP client, the `GET/POST /api/auth/ldap` routes, the Settings-tab panel, the sealed bind password and the `ldap_enabled`/`ldap_login_mode` config were all removed; operators authenticate with the local admin account plus optional TOTP. `github.com/go-ldap/ldap/v3` and its now-unused transitive dependencies were dropped from `go.mod`.

### 🐛 Fixed
- **DoT (853) and DoH (8443) now come up on a domain install whose certificate lives in the built-in ACME store.** The boot-time TLS loader that feeds the encrypted DNS listeners probed `/etc/letsencrypt/live/<domain>/`, the configured `cert_path`/`key_path`, and the generic `certs/cert.pem` — but never `certs/acme/<domain>.crt`, the pair the embedded ACME client writes and the web panel itself serves. An install (or a migrated one) whose config pointed at a path that does not exist therefore exhausted every candidate, logged `TLS configuration error … no CA-signed certificate covers it`, and left both ports dark while the panel stayed up. The loader now probes the ACME pair at the same resolved directory the rest of the daemon uses.

---

## 🔧 [Unreleased] — Preset Catalogue Removed, Bulk Domain Import Added

This fork ([digitrob/DigitDNS](https://github.com/digitrob/DigitDNS)) tracks upstream from v2.8.0-beta.1 with one deliberate divergence: the built-in policy preset catalogue is gone, and routing is entirely the operator's own domain lists.

### ✨ Added
- **A dedicated "Custom Proxied Domains" page** (`/proxied`) with its own entry in the sidebar and the mobile bar, between Dashboard and Clients. Each domain carries a switch: turning one off keeps it listed (dimmed) but lifts it out of the proxy set, so it resolves normally; turning it back on restores proxying. The add box and bulk import move here with it.
- **Bulk domain import.** The Custom Proxied and Custom Blocked lists can be filled a whole list at a time: paste names into the dashboard dialog or upload a file (≤ 1 MiB). Entries may be newline-, comma- or semicolon-separated, `#` starts a comment, `*.` wildcards are accepted, and the import reports how many names were added, already listed, and invalid.

### 🗑️ Removed
- **The built-in preset catalogue** — the 171+ game/streaming/service presets, their embedded JSON (`presets/`), the `internal/presetupd` signed update channel, the bundled policy-icon set, the preset generator tools, and the preset CI workflows.
- **The "Gaming Policies" dashboard tab and every preset switch**, the quick-profile buttons, the per-client policy picker, and the "Policy Catalog Updates" card.
- **The `config.json` `rules` block and the `enable_*` keys** — routing lives in the database and is edited from the dashboard, not seeded from the config file.

### 🔄 Changed
- `GET /api/v1/policies` (and v2) now answers an empty `catalog`; `POST` stores a rule entry (the five `custom_*` keys, including the new `custom_proxied_disabled`) instead of toggling a preset. `custom_proxied_disabled` holds proxied domains the operator switched off on the proxied page: they are persisted and echoed back, but never handed to the live resolver.
- `custom_policies` on a client is accepted and stored but is **inert** — the resolver applies one rule set to everyone.
- The `update-presets` console command is gone and the TUI menu is renumbered; the `/rules` and `/policy` SPA paths still resolve so old bookmarks keep working.

### ⚠️ Note for future merges
- The upstream changelog below is kept as **history** — those releases really did ship the preset catalogue. When merging a newer upstream tag, drop the preset-specific changes rather than reintroducing them.

---

## 🔐 [v2.8.0-beta.1] — Signed Self-Update, Atomic Operator Writes & an Audit Pass

Codename **HyperFORGE**. A security release responding to an independent security audit of v2.6/v2.7: the update channel is now cryptographically signed, every operator write path is atomic, and the two "answered success but didn't persist" anti-patterns left over from the v2.6 pass are gone.

### 🔐 Security
- **The self-update channel is signed (audit #1, HIGH).** The dashboard updater used to compare the downloaded binary's SHA-256 against a `checksums.txt` fetched from the same origin — an unauthenticated self-consistency test, while the docs and the progress message called it "signed". Now every release bundle carries a **detached ed25519 signature over `checksums.txt`**, the public key is pinned in the binary (the private half lives only in the `RELEASE_SIGNING_KEY` Actions secret — the same pattern as the preset channel), and the signature is verified over the raw bytes *before* the hash is even parsed. An unsigned or wrongly-signed bundle is refused, whatever it claims. The updater also **never honours a proxy** (a locally-trusted TLS-intercepting middlebox can no longer be walked into), **re-hashes the file immediately before installing** (so the renamed bytes are provably the verified bytes), and selects releases by **exact version match** rather than substring, so "apply 2.6.0" can no longer install a `v2.6.0-beta.9`.
- **Operator writes no longer erase a subscriber's live bind (audit #3, MEDIUM).** The nine operator/sweep paths — update client, regenerate UUID, set/remove IP, renew, toggle, regenerate the register secret, the expiry sweep, and both traffic-cycle resets — used to read a record in one transaction and save the whole record in a second. A bind committing in between was silently overwritten (the subscriber lost DNS; the freed IP became claimable; the sweep variant could leave an account disabled with no self-heal). They now all run through one `DB.UpdateClient` read-modify-write, so check and commit are atomic — the same fix `registerIP` already had.
- **The access-mode and policy-rules saves stop lying (audit #4/#6).** Both persisted their state, discarded the error, pushed the change to the live resolver and answered `{"success":true}` — so a failed store write meant the rule/mode was live until the next restart, then silently reverted (for `allow_all`, re-opening the resolver to the public). Both now persist first, answer **500** when the write fails, and only then apply to the live state; the dashboard shows the failure and snaps the switch back to the server's real value.
- **The SVG icon `url()` guard is case-insensitive and scans every occurrence (audit #5, LOW).** The re-encode allowlist was sound, but one guard was a case-sensitive prefix test, so `fill="URL(https://…)"` and a second `url()` after an internal one were stored verbatim. The guard now inspects every `url()` in the post-decode value, tolerates quotes and whitespace, and accepts only internal fragments.
- **A wrong-typed migration marker now fails closed (audit #7, LOW).** A marker that is valid JSON of the wrong type (a number, an array, an object with non-boolean members) used to pass the boot-time settings validation and then make the bootstrap migration *skip* its `allow_all=false` convergence, booting a legacy install as an open resolver. The `migration` key is now schema-validated (corruption refuses the boot) and the migration itself distinguishes "absent" from "unreadable" and converges to the safe default either way.

### 🖼 Fixed
- **A runtime domain change now refreshes the resolver's self-name set.** The set that lets a locked-out subscriber resolve the portal link was installed only at startup, so changing the panel/subscriber domain left the retired name answering and the new one refused until a restart. Saving the subscription settings re-applies it, and the panel-domain rebind does too.
- **The SSE ticket store is bounded** at 64 outstanding tickets per session (with a 429 over the cap), so the panel cannot grow unbounded memory in the process that also serves DNS.
- **A weak admin password is refused at boot** (under 12 characters) and a legacy `config.json` still carrying the placeholder `jwt_secret` is called out in the log — two root-cause guards from the audit's history finding.

### 🧪 Quality gates this release passed
- `go build ./...` (host + linux cross), `go vet ./...`, all packages green with `-count=1`; `web/js/app.js` passes `node --check`.
- New tests: signature accept/reject (wrong key, tampered body, missing/truncated/garbage signature), exact-tag matching, the no-proxy transport, atomic `UpdateClient` (fresh record returned, callback error rolls back, missing id refused), the wrong-typed migration marker failing closed in six shapes, the SSE-ticket cap and its 429, and every external `url()` spelling being rejected while internal ones survive.

### ⚠️ Upgrade notes
- **The updater accepts only signed releases from this line onward**, and older installs asking *this* daemon to update them will download signed bundles — no action needed. A release published before the signing step exists cannot be installed by a v2.8+ daemon; use `install.sh` for those.
- The signing keypair is new for this release: set the `RELEASE_SIGNING_KEY` repository secret (base64 of the raw 32-byte ed25519 key) before publishing a tag. `tools/releasesign` verifies signatures with the exact pinned key the daemon uses.

---

## 🎨 [v2.7.0-beta.1] — Real Policy Icons, Custom Group Icons, Working Theme URL Source

Codename **HyperFORGE**. An identity release: policy cards carry their real service logos, custom groups get operator-uploaded icons, and the portal theme feature loses the two defects that kept parts of it invisible.

### ✨ Added
- **Real icons for the built-in policies.** Every policy card now shows an embedded SVG: the actual service logo for the brands it names (Riot, Epic, Steam, PUBG, EA, Battle.net/Blizzard, Ubisoft, Rockstar, PlayStation, Roblox, Discord, Spotify, Twitch, Kick, Google, SoundCloud — sourced from the CC0-licensed [Simple Icons](https://simpleicons.org) collection) and a matching Feather glyph for categories that have no brand (AI, Social, Adblock, FamilySafe, Downloads, and the extra groups). All thirty are baked into the binary, coloured with `currentColor` so they follow the panel's theme, and served at `/icons/policy/<id>.svg` with a Feather fallback if a file is ever missing.
- **Custom icons for custom policy groups.** Each named group can now carry an uploaded icon, set from the group editor: **SVG ≤ 16 KB or PNG ≤ 64 KB (≤ 512×512)**. Uploads are stored in the encrypted database — so the existing backup/restore flow covers them — and the editor shows a live preview with a remove button.
- **A private validator for hand-made presets.** `go run ./tools/presetchannel -in presets validate` now also checks the toggle-key convention and cross-file key duplicates, the action kind, the icon-field convention and the icon file's presence, the RFC 3339 `updated_at` format, `sort_order` collisions, and unreferenced icon files. Repo-only — nothing here ships in any bundle — and the same check gates preset PRs in CI.

### 🖼 Fixed
- **The URL source of the custom portal CSS never loaded.** The static Content-Security-Policy (`style-src 'self' 'unsafe-inline'`) blocked the cross-origin `<link>` the URL source emits, so no external stylesheet ever applied. The portal's CSP now widens `style-src` with exactly the configured origin — the dashboard and API keep the strict static policy.
- **The 16 KiB inline-CSS render bound is now actually enforced.** It was declared but never consulted; a database written before the save-time cap existed could serve an unbounded stylesheet to every portal page. Oversized inline CSS is skipped with a log line.

### 🧪 Quality gates this release passed
- `go build ./...` (host + linux cross), `go vet ./...`, all packages green with `-count=1`; `web/js/app.js` passes `node --check`.
- New tests: the SVG allowlist re-encoder (dirty payloads in, clean geometry out), the PNG validator and format sniffing, every preset having a theme-following embedded icon, the icon route's content type / 404 / method-guard surface, and the portal-only CSP widening.

### ⚠️ Upgrade notes
- No data migration; the new `custom_group_icons` bucket is created on first icon upload and rides along with existing backups. Group icons are panel-admin content only and never appear on the public portal.

---

## 🚀 [v2.6.0-beta.1] — In-Panel Updates, Self-Resolving Service Names, NAT-Aware Installer & a Security Pass

Codename **HyperFORGE**. A reachability-and-maintenance release: the dashboard can update itself, the resolver always answers its own service names, the installer stops guessing the wrong public IP, and a round of audited security fixes lands — including a release-blocking dashboard defect that shipped in v2.5.

### ✨ Added
- **One-click updates from the dashboard.** The panel reads the version published on the project's `main` branch and, when a newer one exists, shows an **Update** badge. Clicking it opens a progress modal that downloads the matching release binary, **verifies its SHA-256 against the release `checksums.txt`**, backs up your data, swaps the binary atomically (keeping the previous one as `.bak`), and restarts onto the new version. Admin-only; Linux/systemd installs only (elsewhere it simply reports that a newer version exists).
- **The resolver answers its own service names ahead of the access whitelist.** A subscriber whose IP changed used to fall off the whitelist and get `REFUSED` for *everything* — including the portal link they needed to re-register from. HyperDNS now answers its own panel / subscriber-portal / DoH-DoT hostnames with the server's public address for any source (still behind the rate limiter, only `A`/`AAAA`, only the server's own IP), so that door stays open without turning the resolver into an open one.
- **Manual public-IP entry at install time.** On NAT'd VPSes — Iran-routed boxes especially — the echo services report a foreign address (Azerbaijan/UAE), which then breaks certificate issuance and subscriber links. The installer now shows the detected IP and lets you correct it (or set `HYPERDNS_PUBLIC_IP` for an unattended run), and writes the chosen value into `config.json` so the daemon uses it verbatim instead of re-detecting the wrong one. Mirrored across all three installers.
- **Required-ports preflight.** The installer lists the ports HyperDNS needs (53, 443, 80, 8443, 853, plus the random panel port) and warns when one is already taken by another service — a local `masterdns`/dnsmasq on 53 is the usual culprit — before it installs anything. The README carries the same table.

### 🔐 Security
- **The forwarder no longer sends the client's transaction ID upstream (HIGH).** Echoing the client-chosen ID to a plaintext UDP upstream donated half the entropy an off-path spoofer needs (RFC 5452); each upstream exchange now uses a fresh random ID and restores the client's ID on the reply, so a whitelisted subscriber can no longer blind-spray a forged answer into the shared cache.
- **DoH token changes apply live (MEDIUM).** Saving or revoking DoH bearer tokens in the dashboard took effect only at the next restart, and a failed save still reported success. The gate now updates in place (mutex-guarded) and the persist error is surfaced.
- **Atomic IP binding (MEDIUM).** The registration read-modify-write is now a single transaction, closing two races: a bind in flight could revert an operator's concurrent account suspension, and two subscriptions could both claim the same address (Mantis C-04).
- **Slow-body read deadline (MEDIUM)** on the web listeners (the SSE stream is exempt so it is not cut off), and the **DoH `GET ?dns=` now enforces the same 4 KiB cap as `POST` (LOW)**.

### 🖼 Fixed
- **The v2.5 dashboard bundle failed to parse (release-blocking).** A function header dropped during v2.5 left `web/js/app.js` syntactically invalid, so the entire dashboard script was rejected by the browser. Restored — the dashboard runs again.
- **Custom subscriber-portal CSS now actually renders.** The v2.4 feature was inert: the stylesheet was emitted into the page head as a `template.CSS` value in an HTML element context, where `html/template` escapes it, so the `<style>` came out as visible `&lt;style&gt;` text and no theme ever applied. It is now typed `template.HTML` so it renders live, and `SanitizeThemeCSS` was hardened to also strip escape-obfuscated external `url()` (e.g. `url(\68 ttps://…)`) now that it is the primary control. See **[docs/TUTORIAL.md → Brand the subscriber portal with custom CSS](TUTORIAL.md#5c-brand-the-subscriber-portal-with-custom-css)**.

### 💾 Data preservation
- Updating never rewrites `data.db` / `master.key` / `config.json`; the in-panel updater snapshots them into `<data-dir>/backups/` before it swaps the binary. A 2.2 → 2.6 in-place upgrade keeps every subscriber, the admin credential and all settings (covered by a new reopen-and-migrate test).

### 🧪 Quality gates this release passed
- `go build ./...` (host + linux cross), `go vet ./...`, all packages green with `-count=1`; `web/js/app.js` passes `node --check`; all three installers pass `bash -n`.
- New tests: upstream transaction-ID regeneration, live DoH-gate application, concurrent same-IP bind resolving to exactly one winner, bind-on-suspended refused without re-enabling, the DoH GET size cap, self-service name resolution, data preservation across reopen+migrate, and the version-compare logic behind the update check.

### ⚠️ Upgrade notes
- No data migration; an upgraded install behaves identically until you opt into anything new. The in-panel updater is Linux/systemd only. The stable line new users land on stays the pinned `v2.2.0-beta.1` — this `-beta` tag is published as an **UNSTABLE prerelease** and does not take the *Latest* badge.

---

## 🌐 [v2.5.0-beta.1] — Per-Subscriber Device Limit & IPv6 for Proxied Names

Codename **HyperFORGE**. Two networking features that had been on the roadmap since the fork feedback.

### ✨ Added
- **Per-subscriber device limit (1–5).** Each account can now hold more than one bound source IP. Set **Max Devices** on the add/edit client form (or `max_devices` in REST v2); a new address past the limit evicts the oldest, LRU-style. The default is 1, so accounts created before this change behave exactly as before. The cross-account address guard (Mantis C-04) is unchanged: an address already bound to a *different* subscription is still refused, so a device limit is not a way to share one paid account across households.
- **IPv6 answers for proxied names.** When the server has a reachable IPv6, set it as **`public_ipv6`** and proxied names answer `AAAA` with it, so an IPv6 client reaches the SNI proxy over v6 instead of being pushed onto IPv4. With `public_ipv6` empty the previous behaviour stands — `AAAA` on a proxied name is withheld so the client uses the proxied `A` record and cannot leak past the proxy over v6. Direct (non-proxied) names already return their real `AAAA`, and the DNS and SNI listeners already bind dual-stack, so this closes the remaining gap for the proxied path.

### 🧪 Quality gates this release passed
- `go build ./...`, `go vet ./...`, gofmt clean; all packages green with `-count=1`.
- New tests: `MaxDevices` clamp (1..5), multi-device LRU eviction, the default single-device path, and the C-04 conflict still refused under a multi-device account; the client codec and settings round-trip guards updated for the new fields.

### ⚠️ Upgrade notes
- No migration: `max_devices` defaults to 1 and `public_ipv6` to empty, so an upgraded install behaves identically until an operator opts in.

---

## 🎨 [v2.4.0-beta.1] — Custom Portal CSS from a File or URL

Codename **HyperFORGE**. Adds a way to brand the subscriber portal beyond the inline CSS box.

### ✨ Added
- **The subscriber portal's custom CSS can now come from a server-local file or an online URL,** not just the inline textbox — inspired by how panels like 3x-ui let an operator point at a stylesheet. Choose the source under **Settings → Subscription Portal → Portal custom CSS**:
  - **Inline** — paste CSS as before.
  - **Local file** — an absolute path on the server (e.g. `/root/css/sub.css`); the daemon reads it (read-only, capped at 256 KiB), runs it through the same sanitiser as the inline box (`<style>`/`<script>` defanged, `@import` and external `url()` stripped), and inlines it.
  - **URL** — an `http(s)` stylesheet address emitted as a `<link>` the subscriber's browser loads directly. The daemon never fetches the URL itself, so the resolver takes on no SSRF or latency risk.
  Only the field for the selected source is stored; a missing local file or a bad value renders the portal with no custom CSS and logs why, rather than failing the page.

### 🧪 Quality gates this release passed
- `go build ./...`, `go vet ./...`, gofmt clean; all packages green with `-count=1`.
- New tests: source resolution for inline/local/url (including sanitisation of a local file's `@import` and a missing-file no-op) and save-time validation of the source selector, the absolute-path rule and the http(s) URL rule.

### ⚠️ Upgrade notes
- Existing inline CSS keeps working unchanged: a record with no source set resolves as `inline`, exactly as before.

---

## 🔵 [v2.3.0-beta.1] — Auto-Updating Presets, Custom Policy Groups, Installer & Login Fixes

Codename **HyperFORGE**. A feature release (new user-facing surface, so a MINOR bump per the project's tag policy).

### ✨ Added
- **A signed preset-update channel.** The 171+ built-in policy presets moved from hard-coded Go slices to embedded JSON data files (`presets/<id>.json`), and the daemon can now pull newer routing lists from a signed channel served over GitHub Pages. Every update is an ed25519-signed manifest (the public key is baked into the binary; the private half lives only in the release workflow's secrets) with a sha256 for each policy file. The daemon downloads only what changed, swaps it in atomically, health-checks the result (probe queries + a minimum-size floor) and **rolls straight back on any failure**; a stale mirror can never downgrade a running catalog, and only domains of policies the binary already knows are updated. Drive it from **Policy Presets → Policy Catalog Updates** in the dashboard, `hdns update-presets`, or menu item 12 in the console. Auto-apply is opt-in per server, with the same health-check and rollback.
- **Named custom policy groups.** Define a named bundle of domains with one action — proxy, direct, or block — and toggle or edit it as a unit, the structured evolution of the flat Custom Proxied/Blocked/Direct lists. Managed from a new **Custom Policy Groups** card in the dashboard and the `/api/custom-groups` endpoints; stored in their own database bucket and re-applied to the resolver on every edit and at startup.

### 🖼 Fixed
- **The console's Uninstall no longer points at a missing script (GitHub issue #1).** A piped `curl | bash` install had no local `scripts/` directory to copy `uninstall.sh` from, so `/opt/hyperdns/scripts/uninstall.sh` was never written and the console's Uninstall failed with a bare *not found*. The installer now downloads `uninstall.sh` the same way it already fetches `restore.sh`, and the console falls back to the built-in cleanup (which also retires the resolver override and firewall rules) when the script is absent.
- **No more pre-login flash of the dashboard.** The panel is a bearer-token SPA, so a browser navigation to the dashboard carried no credential and the shell HTML rendered for a few hundred milliseconds before the JS noticed there was no token and redirected — a pre-auth flash of panel chrome to anyone who knew the admin path. Login now also sets an HttpOnly, admin-path-scoped, `SameSite=Strict` document cookie, and the SPA document route redirects to the sign-in page server-side when it is missing or dead, so the shell never reaches an unauthenticated browser. The cookie gates only the document; every `/api/*` call still requires the bearer header, so no CSRF surface is added.

### 🧪 Quality gates this release passed
- `go build ./...`, `go vet ./...`, gofmt clean; all packages green with `-count=1`.
- New tests: preset channel (sign/verify/apply/rollback/downgrade/restart), custom-group matcher precedence and catalog-swap survival, custom-group service CRUD + validation + restart reload, the login document gate, and the installer download path.

### ⚠️ Upgrade notes
- Existing presets keep working unchanged — the embedded baseline is the permanent offline fallback, and the channel is opt-in. No data migration is required; the new `custom_groups` bucket is created on first write.

---

## 🔐 [v2.2.0-beta.1] — Whitelist by Default, Built-In ACME, API v2, Google/AI Presets & a Real Console

### ✨ Added
- **Client Access Whitelist is the default.** An unknown source is refused by DNS (`REFUSED`, with an RFC 8914 *Prohibited* Extended DNS Error for OPT-bearing queries so the client learns *why*) instead of being served for free. Fresh installs ship whitelist-on; existing installs converge through a one-time idempotent migration, and an operator who wants public mode back flips the dashboard toggle (their choice then persists over every default).
- **Certificates are issued by the daemon itself.** The certbot/acme.sh shells are gone, replaced by an in-process ACME client (`golang.org/x/crypto/acme`, no new dependencies, ships inside the single binary — an offline install can issue the moment it has internet). The HTTP-01 challenge is served out of the port-80 listener that is already running, so issuance and daily renewal happen **while the service runs** — no more stopping HyperDNS so certbot could borrow port 80. Renewed certificates hot-swap into the live listeners without a restart.
- **Google, AI and Social service presets.** Three new policy categories: *Google Services* (Search, Gmail, Drive, YouTube — with the shared CDN zones deliberately excluded and the update CDNs forced direct), *AI Assistants & Platforms* (Copilot, Perplexity, Grok, DeepSeek, Mistral, OpenRouter, …), and *Social & Messaging* (X, Instagram, Facebook, WhatsApp, Telegram, Reddit). Default on like every other proxy category, independently toggleable, per-client selectable.
- **REST API v2.** A versioned successor contract: symmetric field names (`display_name`, `allowed_ips`, `validity_days` — create and update finally agree), cursor-paginated lists (`{items, next_cursor}`, bounded limit), RFC 9457 `application/problem+json` errors, `POST /clients/{id}/actions/{action}` for mutations, and **strict decoding** — an unknown field is a 400, not a silently dropped typo (v1's decoder turned `expires_days` into a lifetime account). v1 keeps working with `Deprecation`/`Sunset` headers.
- **The created-client credentials popup.** Creating a subscriber now opens a dialog with the register link and the registration secret (masked until revealed, copy buttons on both, focus-managed like every other modal) — the two things the operator needs to hand over at the moment they are minted, instead of hunting through the card's Bot Card afterwards.
- **A console worth opening.** `hdns` now clears and redraws in place (no more menu-copy-per-action scrolling), carries the banner and ANSI colors (auto-disabled for pipes, `NO_COLOR` and dumb terminals), sanitizes untrusted names against ANSI injection, prints the subscriber list as an aligned table, and — the functional gap — can **stop and start the service**, plus stop demands a typed confirmation.
- **Configurable game-relay ports and a random management port.** The four extra listeners (5223/5222/2099/8393) are settings, not literals. Fresh installs draw the panel port randomly from 20000–60000 (8080 is everyone's default and collides with half the internet), open it in the firewall, and print it in the banner; upgrades keep their existing port. Every port change — dashboard, TUI or control socket — runs the same shared validator (listener collision, disabled-service warning, privileged-port warning, live bind probe) before anything persists.
- **DoH URLs tell the truth.** A canonical `doh_url` builder (domain-first, the DoH listener's own port, https) feeds the Connect Guide and `/api/config` — the guide used to print `http://IP:web_port/dns-query`, wrong in scheme, port and host at once. The portal's copy buttons copy the displayed values (they hardcoded 8443/853).

### 🔐 Security
- **The master REST key no longer opens the dashboard.** It used to authorize every admin route — settings, credentials, LDAP mode — so one leaked REST key (the credential that lives in integrations and CI logs) was a full panel takeover. It now authorizes the versioned REST API and exactly one dashboard route: the lockout-recovery unlock the root-local TUI calls. Dashboard routes take real sessions.
- **The public subscription API no longer leaks the registration secret or operator note** (critical v2.1.0 fix carried in this release's baseline: `GET /api/sub/<token>` served the full client record including the write credential; it now serves display data only).
- **The DoH listener no longer serves the admin panel.** The DoH port answered with the complete dashboard handler — login page, admin namespace, every `/api` route — on the wildcard bind, so any network that could reach the DoH port could reach the panel's admin surface too. It now serves `/dns-query` plus the same public portal surface the subscriber links name, and the same bare 404 for everything else.
- **v1 API-key rotation faces the second factor.** `POST /api/v1/api-key` rotated the master key with no TOTP check while the dashboard's rotate endpoint demanded one — with 2FA enabled, a hijacked session could rotate the key through the v1 route unguarded. The same gate now runs on both (injected into the API router; still a no-op while 2FA is off).
- **The port validator covers the HTTP→HTTPS redirect listener.** The shared validator knew the DNS trio and the SNI relays but not the redirect port, so a panel port that collided with it was only discovered as a fatal startup in service mode — the exact failure the validator exists to prevent.
- **The directory can no longer be repointed from a stolen session.** Saving the LDAP settings now re-asks for the current password (and a current code when 2FA is on) — the directory IS the login path, and repointing it was the one credential change a bare session could still make. And the panel now decides who may log in: only the directory entry naming the admin account authenticates, where before every user under the configured BaseDN came in with full panel admin.
- **API-key rotation re-asks for the current password**, on the dashboard — matching the file's own invariant ("look, but not touch"). A validated TOTP code is also **single-use across every gated endpoint** now (login included), so one observed code cannot authorise a rotation, an unlock and a login in its validity window, and the code no longer rides the query string on the v1 route.
- **The pending 2FA enrollment secret is no longer readable through a live session.** `/api/auth/2fa/status` answers with a bare "enrollment open" marker; the URI (which embeds the secret) only leaves the password-gated setup response.
- **The keyless metadata routes face the same bind gate.** `/api/v1/version`, `/api/v1/docs` and `/api/v2/version` answered external callers even when the REST API was bound to localhost; they now 403 like everything else on a loopback bind.

### 🖼 Fixed
- **The first DoT/DoH certificate now goes live.** A fresh install that issued the DoH/DoT domain's certificate from Settings persisted the domain, skipped the nil certificate holder, skipped the listener rebuild (a flip flag nothing ever set), and logged that the listeners served the new certificate while they kept serving the panel's until restart. The holder is now constructed mid-process and the listeners rebuilt the moment the issuance lands; clearing the domain detaches the dedicated source just as completely.
- **The DoT/DoH listener rebuild is serialized.** Two concurrent dashboard saves (or a save racing the daily renewal) could interleave the shutdown/start of the TLS listeners and race on their server fields. A mutex covers the source swap and the rebuild.
- **A failed portal rebind no longer leaves the record moved and the portal down.** Issuing a subscription certificate persisted the new domain first and rebound the listener second — a bind failure (port conflict) kept the new record while the portal listener was gone. The previous record is now restored and rebound, and the response says so.
- **ACME robustness.** The HTTP-01 challenge answer strips a default port from the Host header (a validator sending `Host: name:80` no longer misses the pending lookup); the account key is read for persistence through a locked accessor instead of a bare slice-header read; and the client-setup network calls run outside the manager mutex that the port-80 challenge path shares, so a slow registration can no longer stall challenge answering.
- **A stalled live-log viewer can no longer pin the stream.** The SSE stream runs without a global write timeout by necessity, but each frame now carries its own write deadline and a failed write releases the subscriber slot — a client that stops reading used to hold both the goroutine and one of the capped slots until restart.
- **Every certificate the daemon issues now renews itself.** The daily renewal loop checked only the panel domain, so a subscription portal or DoH/DoT certificate hit its expiry date around day 90 with nothing coming. The loop now covers all three surfaces, and a panel domain whose boot issuance once failed is retried daily too. A run that fails after the certificate lands (persist, validation, apply) now marks the progress bar terminal instead of leaving it at 90% with no error, and a failed DoT/DoH listener rebuild is reported to the operator instead of logged as success.
- **A deliberately customised portal port survives panel-port changes.** The dashboard now marks a port the operator chose (and unmarks it when they return to the panel's), so the boot-time drift repair and the live copy-follow only re-point records that still ride the panel origin — and the repair also recognises the cleared-domain shape the current UI can produce.
- **A negative quota or validity can no longer invert a plan.** `traffic_limit_gb: -50` behaved as unlimited and `days: -5` as lifetime; both are refused at creation and on update.
- **v2's strict decoder covers the frame boundary.** Trailing JSON after the first value (`{"a":1}{"b":2}`) was accepted on the first object with the second silently dropped; it is a 400 now.
- **API examples use the configured domain, not the IP.** The panel's curl/Python/Node snippets (and the Node snippet's `window.location.origin` — undefined in Node — replaced by a real `ORIGIN` constant) now speak the domain, the real scheme and the real port, against `/api/v2`.
- **v1's login-2FA reveal ordering** (carried in this release's baseline): the dashboard login form consumed the response body before reading the `twofactor_required` flag, so the 2FA code field never appeared and a correct password looked like a wrong one.
- **`allow_all` can no longer be silently overridden by config.json** at every boot (DB-wins precedence restored), and the v2.2.0 migration respects an operator's later explicit choice.
- **The installer's final banner no longer tells the operator to stop the service before opening the terminal console.** The old wording ("TUI and daemon cannot share the database at once") was stale v2.1.0 guidance: since v2.2.0 the `hdns` console is a pure control-socket client — it talks to the running daemon and never opens the database, so it runs beside the service at any time (`hdns status` and `hdns flush` likewise). Fixed in the online installer, the offline installer and the offline bundle.

### 🧪 Quality gates this release passed
- `go build ./...`, `go vet ./...`, gofmt clean; **19/19 packages** green with `-count=1`.
- New regression suites: whitelist migration (converge-once, idempotent crash-window rerun, operator-choice respected), EDE attachment (OPT-only, quota refusals not mislabelled), ACME challenge path (unknown token / foreign Host / non-GET relay unchanged), preset resolution (shared-CDN exclusion), v2 contract (pagination bounds, strict decode, symmetric names, action routes), v1 deprecation headers, and master-key scoping.
- Installers: `bash -n` clean; the offline bundle mirror is byte-identical to the canonical copy.
- Linux install verification and the race detector run in WSL Ubuntu-24.04 (see the release notes for the result).

### ⚠️ Upgrade notes
- The first boot after upgrading applies the whitelist migration: **unregistered sources stop resolving.** Register clients (or re-enable public mode from the dashboard) deliberately.
- Any integration holding the master key **must keep using `/api/v1` or `/api/v2`**; it can no longer drive dashboard admin routes.
- The offline bundle no longer ships `scripts/ssl_issue.sh` — there is nothing to run by hand; the daemon issues and renews on its own.

---


### ✨ Added
- **The subscriber port is a real listener now.** The Subscription Portal's port field used to change only the *text* of the links it generated — nothing ever bound it, so an operator who set a portal port published a URL that refused the connection. Saving the card now binds a dedicated public listener (and moves it live on the next save, no restart). That listener serves **only** the portal routes — `/sub/`, `/ip/`, `/api/sub/`, `/css/portal.css`, `/js/portal.js`, `/fonts/` — and answers the admin namespace, the REST API, DoH and the SPA with the same bare 404 a stranger gets. Its TLS scheme follows the panel's, reusing the panel certificate by default or the record's own pair for a separate subscription domain, so the link's scheme always matches what is listening.
- **A per-reason registration answer.** `POST /ip/<token>` now returns a machine-readable `reason` beside its English text (`secret`, `suspended`, `expired`, `quota`, `conflict`, `invalid`), and the portal renders its own translated line for each. The subscriber's copy is in both languages, and the reasons are only reachable **after** the secret check — a wrong-secret probe cannot tell a taken address from a free one.

### 🖼 Fixed
- **The footer is inside the content column.** The root cause was not CSS: a `<div class="glass-panel">` in the REST API tab was never closed, and an extra `</div>` in the Settings tab balanced the count while closing the layout wrapper early. The HTML parser then reparented the Connect tab, the desktop footer and the mobile nav onto `<body>`, which is a row-wise flex container — so the footer rendered **beside** the content, at the right edge of the page. Both tags are corrected, and `index.html` now carries structural regression tests (per-tab div balance, footer-inside-main, no mojibake, no control bytes) so the next hand-edit that unbalances a tag fails the build instead of reaching a screenshot.
- **`Reg Link` hands out the subscriber page, not the API.** The button copied `.../ip/<token>` — an API endpoint since Phase B — while `/sub/<token>` is the page a subscriber opens. It now copies `/sub/`, and a separate `data-reg-url` carries the registration API for the Bot Card.
- **The Bot Card is current.** The Telegram text named the retired `/ip/` link as if it were the subscriber page and printed a bare `your dedicated DNS:` heading with nothing after it. It now carries the four things the subscriber needs: the `/sub/` portal link, the registration secret, the DNS address, and the automation API URL.
- **The Portal title reaches the page.** The h1 rendered a translated constant, so the operator's Portal title changed only the browser tab. The heading is now their `Title` (falling back to "HyperDNS"), and the subtitle and footer carry the **running** version instead of a hardcoded `v2.0.0-beta [HyperRAIN]` that had survived two releases.
- **A failed listener bind is reported, not swallowed.** If the portal port is already held, the save persists and the card says so with the reason, rather than claiming success over a port nothing can reach. The card also names the one manual step the panel cannot take for the operator: opening that port in the host firewall.

### 🧪 Quality gates this hotfix passed
- `go build ./...` clean; **17/17 packages** green with `-count=1`; `go vet` clean.
- **Race detector 17/17** inside an isolated container (`golang:1.26-bookworm`, `--network=none`, modules pre-seeded in a separate networked phase) — zero DATA RACE reports.
- **Playwright E2E 20/20** against the live demo daemon, including a new **12-test Settings sweep** that exercises every card in that tab — portal save/persist, 2FA, LDAP, admin path, SSL, custom proxied domains, custom blocked domains, DoH tokens, static records, session idle, and the credentials modal — asserting persistence across a reload rather than trusting a toast.
- **Mantis PoC set (5 new, all REFUSED)** for the new public listener: the subscriber port exposes no admin namespace, no auth endpoints, DoH or SPA routes (20 path shapes including traversal and case variants); registration is not a token oracle (a live token with a wrong secret answers identically to an unknown token); the new reason codes leak nothing to a caller without the secret; and the operator-supplied Portal title is escaped, not executed.
- Served-asset verification over the running daemon: the portal answers 200 on its own port with the operator's brand line, version-correct subtitle and footer; the admin routes answer 404 there; `/css/portal.css`, `/js/portal.js` and the fonts answer 200.

### 📦 Artifact
- The offline bundle was rebuilt from the fixed sources. `hyperdns-offline-v2.1.0-beta5-20260910-1701.zip`, sha256 `0a57cb817a7e50a9e7cc5c3c80df07d81798527d5af932ef28585285177a8cee` (the beta4 zip is withdrawn).

---

## 🖼 [v2.1.0-beta Hotfix 4] — Dashboard Text Corruption & Footer Placement

### 🖼 Fixed
- **Every stray `â`/`Â` is gone.** The shipped `index.html` carried cp1252-mojibake text baked into the embedded document: em-dashes, bullets, curly quotes, the setup-wizard `●` glyph and the Telegram template emojis were all double-encoded. Every sequence in the document was repaired back to proper UTF-8 (140+ runs across the static markup and the inline Telegram templates); the served document now scans **zero** mojibake markers and **zero** stray control bytes, and the portal's server-rendered strings (`portal.go`, `portal_i18n.go`, `landing.go`) were swept with it.
- **The desktop footer sits inside the content column again.** The `<footer>` had drifted outside `</main>`, so the flex layout pushed it to the right edge of the page. It is moved back inside the main element, and the placement is pinned by the served-document checks.

### 🧪 Quality gates this hotfix passed
- `go build ./...` clean; **17/17 Go packages** green with `-count=1`.
- **Playwright E2E 7/7** against the rebuilt demo daemon — the suite now targets that instance's live credentials and re-verified the standalone login, the 2FA hiding, the portal secret gate, and the session-idle card end to end.
- Served-asset verification over the running daemon: all 6 dashboard assets (CSS/JS) and the admin-rewritten document answer 200 with clean UTF-8; the `/login` page serves with the admin base rewritten and no placeholder leak.
- The offline bundle was **rebuilt** from the fixed sources — the previous beta4 zip contained the corrupted document. Artifact at the time: `hyperdns-offline-v2.1.0-beta4-20260910-1053.zip` (superseded by Hotfix 5's beta5 bundle, which also carries the footer-structure fix).

---


## 🔐 [v2.1.0-beta Hotfix 3] — Standalone Login, Secret-Gated IP Registration, Lockout & Session Controls

### ✨ Added
- **A standalone `/login` page (Phase A):** `/<admin-path>/login` serves a self-contained sign-in document — inline CSS, one small script, **zero external asset requests** — so the whole dashboard bundle (index.html + app.js + chart/icon libraries) is no longer readable pre-auth by whoever holds the namespace. The retired `/<admin-path>/dash/login` redirects to it. The two-factor field appears **only** when the server says the password verified but the code failed (`twofactor_required`, Hotfix 2's flag), so a first-time login never shows a code box.
- **Out-of-band registration secret (Phase B, Mantis C-03):** every account now carries a 96-bit `register_secret` (AES-256-GCM sealed beside the token). The subscriber's `POST /ip/<token>` demands it; the operator's client modal displays and regenerates it. A leaked `/sub/` link alone is now **read-only material**.
- **`/ip/<token>` is an API now (the operator's own algorithm):** POST binds a new address (with no `ip` field it binds the caller's — the 1-click flow); GET serves a bilingual explainer page for old bookmarks. `/sub/<token>` is a pure read-only overview: **no visit, prefetch, or unfurler can move a binding any more** (C-01/C-02/C-03 all closed dynamically — the four Mantis PoCs were re-run against the fixed build and all four now refuse).
- **Gates on the registration path:** a suspended account answers 403 ("suspended"), an expired one 403 ("expired"), a quota-drained one 403 (against the **live** ledger figure, not the stale stored counter), and an address already bound to a *different* subscription answers **409 without disabling either account** — CGNAT makes shared addresses ordinary (C-04).
- **Login lockout hardening + operator unlock (Phase C):** 3 failures → 10 minutes (was 5/15). New `POST /<admin-path>/api/auth/unlock` (session or API key, plus a current TOTP code when 2FA is on) clears one or all locked addresses, and the TUI gained menu entry **[12] Clear Login Lockouts** that calls it over loopback — the tracker lives in the daemon's memory, so this works **while the service runs**, with no restart.
- **Configurable session idle timeout (Phase D):** `POST /api/settings` accepts `session_idle_minutes` (5–1440, default 15), applied **live** on the running SessionManager and persisted. New Settings card shows the value in force and saves a replacement. The 24 h absolute lifetime is unchanged.

### 🖼 Fixed
- **`initSubscriptionSettings` ReferenceError (found by the Playwright E2E):** app.js called the function bare, but it lives inside the twofa ES module and was never exported onto `window` — every dashboard boot threw, and `handleRouteFromURL` never ran, so deep links like `/<admin-path>/dash/settings` silently stayed on Dashboard. The module now exports it through the same bridge as the 2FA/LDAP hooks, and app.js calls it via `window.initSubscriptionSettings?.()`.
- The session-idle card lives in the **Settings panel** (tab-rules), where the route actually lands — not in the API tab.

### 🧪 Quality gates this hotfix passed
- `go vet` clean; **17/17 packages** green twice: on the dev box and inside the isolated Mantis container (`golang:1.26-alpine`, `--network=none`).
- **Race detector 17/17** in `golang:1.26-bookworm` — zero DATA RACE reports.
- **Playwright E2E 7/7** against a live isolated daemon: standalone login renders without external requests and hides the 2FA row; a wrong password never reveals it; the correct password lands on the dashboard; the retired route redirects; the portal binds nothing on a plain visit and binds **only** with the correct secret; the session card saves, persists, and re-reads.
- The four dynamic Mantis PoCs (C-01..C-04) re-run in the container: **all four now refuse** where the beta accepted.

---
## 🛠 [v2.1.0-beta Hotfix 2] — Not-Secure Panel & Broken Dashboard Assets

### 🔐 Fixed
- **The "Not secure" panel is gone — root cause closed.** `ValidatePanelCertificate` now refuses a **self-signed** certificate whenever a panel domain is configured. The fallback generator writes the configured domain into its SAN, so the old date-and-hostname-only check treated the self-signed pair as "a usable certificate is already in place" and the ACME paths never fired; the daemon then served the fallback and every browser showed Not secure. The panel now **fails closed** on a domain install without a CA-signed certificate, with an actionable error, instead of serving one no visitor will trust.
- **The 8443 DoH/DoT listener honors the same policy.** `LoadOrGenerateTLSConfig` no longer generates or accepts a self-signed fallback while a domain is configured — DoT and DoH come up only with a CA-signed certificate, instead of silently answering with one every Android Private DNS client and browser refuses.
- **ACME issuance order fixed.** The startup issuance used to run *after* the SNI proxy bound ports 80/443, so certbot `--standalone` could never bind the challenge port on a default install. It now runs before any listener binds and before the TLS config is loaded, so a freshly issued certificate is what the listeners serve on their first start — and it no longer runs for one-shot subcommands (`hdns flush`, `uninstall`).

### 🖼 Fixed
- **Dashboard CSS/JS actually load behind the hidden admin path.** `index.html` was written for the pre-v2.1 root mount and references `/css/…` and `/js/…` absolutely; behind `/<admin-path>/dash/` the browser resolved those against the host root, where nothing has been served since v2.1 — the panel rendered unstyled with every script 404ing. The document is now served with its asset references rewritten below the live admin namespace (per-prefix cached, its own ETag, gzip preserved), and the rewritten references are pinned by regression tests that fetch every one of them through the real handler.

### 📦 Changed
- **Offline installer issues the certificate itself (3x-ui method).** Step 4b runs the bundled `scripts/ssl_issue.sh` (acme.sh, standalone HTTP-01 with the service stopped around the challenge, full chain + key installed, automatic renewal via acme.sh's cron with a restart) while port 80 is still free. It verifies the result is actually CA-signed (issuer ≠ subject) before continuing, retries once for DNS propagation, and **stops with the real cause** (A record not pointing here / port 80 blocked / rate limit) instead of printing a success banner.
- **The success banner is now earned.** Step 6 polls `is-active` for up to 20 s, then probes the real listener over HTTPS with the system trust store (`--resolve`, no `-k`): the dashboard document, one stylesheet and one script must each return 200 through the hidden path. Any failure exits 1 with diagnostics; the celebration only prints when every check passed.
- **Installer polish:** the public IP is detected up front (the domain prompt used to print "Point an A record at " with an empty address), the domain prompt defaults to and validates the existing config's domain, `HYPERDNS_DOMAIN`/`HYPERDNS_EMAIL` cover non-interactive provisioning, mangled `??` banner text replaced with readable labels, and `ssl_issue.sh` no longer passes `--force` (a re-run no longer burns one of Let's Encrypt's five duplicate-certificate slots per name per week).
- **Release pipeline:** the offline bundle now ships `version.json` (the daemon's drift check needs it), and `scripts/install-offline.sh` ↔ `offline-bundle/install.sh` are byte-identical canonical copies.

---

## ⚡ [v2.1.0-beta] UI/UX & Release Hardening (rework Phases 4+6)

### ⚡ Performance
- **Tailwind JIT Engine Removed (−397 KB + no `eval`):** The Play CDN JIT runtime that compiled utility classes in the browser is replaced by a committed, purged 42 KB stylesheet (`css/tailwind.purged.css`) compiled ahead of time by `tools/tailwind.config.js` (dev machine only — no Node at build or deploy time). `script-src 'unsafe-eval'` is gone from the CSP, the page renders styled on first paint without waiting for a compiler, and the class inventory is pinned by `TestEveryUtilityClassSurvivedThePurge` — a class the purge misses fails the suite with the regeneration command.

### ✨ Added
- **Vendored Swagger UI:** `web/swagger/` (Apache-2.0) ships inside the binary; `/api/v1/docs` loads the viewer same-origin and the per-route CSP exception for `unpkg.com` is gone. "100% offline" is now literally true for every asset.
- **First ES Module:** the 2FA/LDAP settings panel (`js/modules/twofa.js`) is the dashboard's first `<script type="module">` — strict mode, deferred execution, reading shared state through the `window.__hdns` bridge at call time.

### 📦 Changed
- Version bumped to **v2.1.0-beta (HyperSHIELD)**; installer banner and UI placeholders updated.

---
## 🔒 [v2.1.0-beta] - 2026-09-07 (Codename: HyperSHIELD)

### 🔐 Highlights
- **HTTPS Panel, Done Properly (Phase 1):** The dashboard listener serves TLS with `tls.LoadX509KeyPair`, validates that the certificate covers the configured domain and is currently usable, and **fails closed at startup** otherwise. An optional redirect listener answers GET/HEAD with `https://…` (405 + `Allow` for anything else, never redirecting attacker-typed hosts). HSTS `max-age=31536000` goes out only on responses that actually arrived over TLS.
- **The Root Domain Is No Longer the Login Page (Phase 2):** `/` now serves a small, self-contained Matrix-inspired landing page — no external assets, no operational data, no links, no credential-adjacent words. It carries a real title, WCAG-AA contrast, a `prefers-reduced-motion` fallback and a `<noscript>` variant.
- **A Hidden Admin Namespace (Phase 3):** The dashboard SPA and every dashboard API moved below a random, once-generated 16-hex-character path (`/<admin-path>/dash/…`, `/<admin-path>/api/…`). Old root routes answer the same plain 404 a random path gets — no redirect ever reveals the namespace. Regeneration is an authenticated, explicitly confirmed operation that invalidates every session and bookmark immediately. The frontend reads the prefix from the address bar, so the same bundle works on every install.
- **Subscription Surface as a Setting (Phase 4):** A dedicated subscription record (enabled, domain, port, title, certificate paths, panel-certificate mode) seeds itself once from the panel origin and never rewrites operator edits. Subscriber links are built from one effective-URL layer instead of scattered string concatenation; a different subscription domain requires a certificate naming it **before** the settings save, not after a restart.
- **Two-Factor & LDAP (Phase 6):** RFC 6238 TOTP with a proper enrollment flow (secret shown exactly once, confirmation requires the code), and directory login through go-ldap with bounded timeouts and verified TLS. LDAP modes: local, ldap, or ldap-with-local-fallback. When 2FA is on, password changes and API-key rotation demand a current code — a stolen session plus a stolen password is no longer enough.

### 🛡 Security
- Login answers "Invalid credentials" identically for local, LDAP, and second-factor failures — no oracle for which path exists.
- The auth record (TOTP secret, LDAP bind password) is sealed at rest by the settings envelope, and structurally cannot serialise into a response: the snapshot type has no field for either.
- Request paths are cleaned mux-style before the admin-prefix check, so traversal cannot escape the hidden namespace.
- Portal pages accept operator CSS only after `<style>`/`<script>` escaping, `@import` stripping, and external `url()` removal — subscriber pages cannot become data-exfiltration surfaces.

### ✨ Added
- `internal/web/landing.go`, `internal/web/security.go` (validators: admin path, domains, ports, CIDRs, time zones, certificates, CSS), `internal/web/subscription.go` (effective-URL layer), `internal/auth` (TOTP + LDAP), `internal/database/subscription_access.go` and `auth_access.go`.
- `POST /api/settings/regenerate-admin-path` (requires `{"confirm":true}`), `GET/POST /api/settings/subscription`, `GET /api/auth/2fa/status`, `POST /api/auth/2fa/{setup,enable,disable}`, `GET/POST /api/auth/ldap`.
- Settings-tab panels: Hidden Admin Path, Subscription Portal, Two-Factor Authentication, LDAP — all in English.

---

## INTERNAL — Fix Bugs

### [v1.3.0-beta - v2.0.0-beta]
- INTERNAL - Fix Bugs
- INTERNAL - Fix Security Bugs

---

## 🚀 [v1.2.0-beta] - 2026-08-31

### 🌟 Highlights
- **171+ Game & Platform Support Catalog:** Integrated an extensive catalog of gaming presets covering Tactical Shooters, MMORPGs, Anime/Gacha, Sports/Racing, Co-Op/Survival, Anti-Cheats, Launchers, and Cloud Gaming platforms.
- **Developer REST API v1:** Comprehensive headless management API with dual authentication (`X-API-Key` & JWT) and interactive Swagger documentation at `/api/v1/docs`.
- **Strict 1-IP Accounting & Expiration Watcher:** Multi-client subscription management with strict single-IP enforcement per account and instant 1-minute expiration deactivation.
- **Auto-Detection & Zero-Loss Safe Installer:** Installer automatically detects previous installations (e.g. `v1.1.0-beta`), creates timestamped backups, and performs non-destructive configuration schema migration.

### 🎮 Added
- **Game Presets & Routing Rules (171+ Titles):**
  - **Tactical Shooters Extra (`enable_shooters_extra`):** Delta Force: Hawk Ops, Escape from Tarkov, HellDivers 2, Rust, Squad, DayZ, ArmA Reforger & 3, Dead by Daylight, Hunt: Showdown, Insurgency: Sandstorm, SCUM.
  - **Anime, Gacha & Modern MMOs (`enable_anime_gacha`):** Genshin Impact, Honkai: Star Rail, Zenless Zone Zero, Wuthering Waves, Throne & Liberty, Path of Exile 1 & 2, Lost Ark, Arknights: Endfield, Warframe, Elden Ring, Black Desert Online, Final Fantasy XIV.
  - **Sports, Fighting & Racing (`enable_sports_racing`):** EA Sports FC 25 / FIFA, eFootball / PES, Street Fighter 6, Mortal Kombat 1, Tekken 8, 2XKO, Assetto Corsa & Competizione, Euro Truck Simulator 2, Forza Horizon 5, F1 24.
  - **Co-Op, Survival & Strategy (`enable_coop_survival`):** Palworld, Enshrouded, Satisfactory, Deep Rock Galactic, Civilization VI/VII, Paradox Interactive (Hearts of Iron IV, Stellaris, Crusader Kings III).
  - **Extra Platforms, Anti-Cheats & Cloud Gaming (`enable_platforms_extra`):** Faceit AC, Riot Vanguard, EasyAntiCheat (EAC), BattlEye, Ricochet, Denuvo, GeForce NOW, Boosteroid, Xbox Cloud Gaming, Razer Synapse, Logitech G Hub, Corsair iCUE.
  - **SoundCloud Preset (`enable_soundcloud`):** Full unblocking of SoundCloud web, desktop player, mobile apps, and audio streaming CDNs.
- **Developer REST API v1 (`/api/v1`):**
  - `GET /api/v1/status`: Live server status, CPU %, RAM MB, QPS telemetry, upstream health, and client stats.
  - `POST /api/v1/clients`: Create new subscriber accounts with custom validity periods and automatic 1-click IP token generation.
  - `GET /api/v1/clients`: List clients with status filtering (`?status=active|expired`) and real-time search (`?search=`).
  - `GET /api/v1/clients/{id}`: Detailed client inspection including allowed IPs, expiration date, query counts, and last seen timestamps.
  - `POST /api/v1/clients/{id}/renew`: Extend subscription duration by N days and instantly reactivate expired accounts.
  - `POST /api/v1/clients/{id}/ip`: Update/bind client IP with strict single-IP limit enforcement.
  - `DELETE /api/v1/clients/{id}`: Terminate client access and instantly revoke DNS resolution.
  - `GET / POST /api/v1/rules`: Inspect and hot-toggle any game or security preset rule without restarting the daemon.
  - `GET /api/v1/diagnostics`: Run on-demand network latency benchmarks against gaming server regions.
  - `GET /api/v1/docs`: Embedded interactive OpenAPI / Swagger API reference.
- **Installer Auto-Detection & Migration:**
  - Auto-detection of existing installations in `scripts/install.sh` and `install-offline.sh`.
  - Automatic timestamped backup creation (`/opt/hyperdns/backups/backup_YYYYMMDD_HHMMSS/`).
  - Safe JSON schema migration that preserves 100% of existing accounts, admin credentials, custom rules, and SSL certificates while injecting newly introduced keys.
  - CLI version flags: `hyperdns -version`, `hdns -version`, `hdns -v`.
- **Documentation:**
  - Added **`docs/PRESET_CATALOG.md`**: Complete categorized directory of all 171+ supported games, publishers, and platforms.
  - Added **`API.md`**: In-depth REST API guide with Python (Telegram Bot integration), Node.js (Provisioner webhook), and cURL integration samples.

### 🔄 Changed
- **Client Accounting Logic:**
  - Upgraded client IP limit from multi-IP array to a strict 1-IP policy per token to prevent account sharing.
  - Added background ticker running every 60 seconds to automatically disable expired client accounts in real time.
- **Web UI:**
  - Reorganized Gaming Policies section on Web Dashboard to include the 5 new preset categories.
  - Updated all dashboard version badges to `beta 1.2.0`.

### 🐛 Fixed
- **UI Render Crash:** Fixed a missing closing brace `}` in `web/js/app.js` within `renderConfig()` when TLS was not yet configured.
- **Expired Account Filtering:** Fixed edge case where pre-expired accounts (`expires_days: -1`) were not correctly flagged during client creation.

---

## 📦 [v1.1.0-beta] - 2026-08-30

### 🎮 Added
- **Multi-Client Access Control:**
  - Token-based client management allowing individual subscription tokens.
  - 1-Click IP auto-registration URLs (`/r/:token`) for end-users to register their dynamic IP without accessing the admin panel.
  - Granular bandwidth and total query counters per subscriber.
- **Interactive TUI Utility (`hdns`):**
  - Global `hdns` terminal management console.
  - Direct subcommands: `hdns status`, `hdns restart`, `hdns logs`, `hdns flush`, `hdns diag`, `hdns clients`, `hdns uninstall`.
- **Live Query Telemetry:**
  - Real-time Server-Sent Events (SSE) live query log stream in the Web Dashboard.
  - Live upload (`↑ KB/s`) and download (`↓ KB/s`) bandwidth meters.
- **100% Offline Installation Package:**
  - Standalone offline bundle with zero internet access requirement (`hyperdns-offline-bundle.tar.gz`).
- **Automated SSL Management:**
  - Let's Encrypt SSL auto-issuance integration via Certbot and ACME scripts.

### 🔄 Changed
- Refactored in-memory routing table for higher concurrency and zero mutex contention.
- Hardened default OWASP security headers for Web UI and REST endpoints.

---

## 🏁 [v1.0.0-beta] - 2026-08-28

### 🌟 Initial Beta Release
- **Core Engine:**
  - High-performance DNS server written in Go supporting UDP/TCP Port 53, DNS-over-TLS (DoT Port 853), and DNS-over-HTTPS (DoH Port 8443).
  - Sharded in-memory LRU cache with sub-millisecond query response times.
  - Fastest Upstream Racing pool querying Cloudflare (`1.1.1.1`), Google (`8.8.8.8`), and Quad9 (`9.9.9.9`) in parallel.
- **Transparent SNI Proxy:**
  - Non-decrypting zero-knowledge TCP proxy on ports 80 (HTTP) and 443 (HTTPS).
  - Anti-DPI TLS ClientHello fragmentation to bypass restrictive deep packet inspection firewalls.
- **Smart Game Presets:**
  - Core gaming presets: Riot Games (Valorant/LoL), Epic Games (Fortnite), Steam/CS2, PUBG, Call of Duty, Supercell, EA, Blizzard, Ubisoft, Rockstar, Xbox Live, PlayStation Network, Roblox.
  - Streaming presets: Discord (Voice RTC + Chat), Spotify, Twitch, Kick.com.
  - Developer 403 bypass: Docker Hub, OpenAI, Anthropic Claude, npm, Gradle, Android SDK, HuggingFace.
  - AdBlock & Family Safe content sinkholes.
- **Cyberpunk Web Dashboard:**
  - Dark neon cyberpunk theme with live QPS graphs, CPU/RAM usage meters, and preset toggles.
