# 📜 DigitDNS Changelog

All notable changes to **DigitDNS** — a fork of the **HyperDNS** project by HyperRAIN — will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## 🏷️ [v5.8.2] — AdGuard custom rules, command palette, performance

### Added
- **AdGuard-syntax custom filtering rules.** A text area on the Ad Blocking card
  (web + Android) accepts AdGuard/uBlock network rules (`||domain^`, `@@||`,
  `$dnsrewrite=`, `$important`, hosts format). Rules compile into the same index
  as subscribed blocklists.
- **`$dnsrewrite` support.** `$dnsrewrite=IP` (A/AAAA), `$dnsrewrite=host`
  (CNAME), `$dnsrewrite=TXT "text"`, and `$dnsrewrite=NXDOMAIN/REFUSED/SERVFAIL`.
- **Command palette.** Ctrl/Cmd+K opens a fuzzy search over tabs, settings
  sections and global actions (theme toggle, sign out) on the web panel.
- **Android status widget.** A 2×1 home-screen widget showing queries today and
  the live query rate; tap to open the app. The dashboard's polling loop keeps it
  fresh.
- **Android toggle widget.** A 1×1 widget showing filtering on/off; tap opens the
  app's Ad Blocking page.
- **Android switches contrast.** Switches in the grey theme now use the success
  colour for the checked track (was nearly white).

### Changed
- **Resolver hot path.** System metrics are sampled once per second in
  `telemetryLoop` instead of on every `/api/stats` call. `normalizeProto` uses
  `EqualFold` (zero alloc per query). Aggregator uses a per-bucket mutex and an
  atomic current-bucket pointer (cleaner design, enables future sharding).
- **Web theming.** Non-theme-mapped `sky-*` and `violet-*` Tailwind utilities
  replaced with theme-mapped `cyan-*`/`purple-*` across DoT settings, custom
  policy groups, the top-upstreams icon, the 2FA card and the zap icon — all
  three themes now render consistently.
- **Adaptive selection.** `adaptiveHealthy` now accepts resolvers that have a
  successful probe, so the Force button switches to the fastest probed resolver
  even if it never served live traffic. The timer resets after each forced run.

### Fixed
- **Upstream Force button.** Not all the time did it switch to the fastest
  resolver (probed-but-unmeasured resolvers were excluded). Fixed.
- **AdBlockingScreen compile.** `LongParameterList` and `MaxLineLength` detekt
  gates kept green after adding the custom-rules field.

### Notes
- No database schema migration is required; existing settings, clients,
  blocklists, certificates, relay agents and egress options are preserved.

## 🏷️ [v5.8.1] — alert pictograms and resolver hot-path

### Added
- **Alert channel pictograms.** Web and Android alert channel rows now show a
  destination pictogram (ntfy, Telegram, Discord, Slack, Gotify, Webhook) in the
  kind picker and on the card, with a check on the active choice; the web
  picker is a custom menu so the glyphs render in dark, grey and light themes.
- **Android alert channel cards.** Each channel is now its own bordered tile:
  the kind pictogram opens the type menu, "Enabled" is labelled, and the channel
  name gets its own full-width row so it can never collapse on a phone.
- **Empty state.** "No channels yet — add one to receive alerts." on both
  clients when there are no channels.
- **Benchmarks.** `BenchmarkOrderedByLatencyWarm`, `BenchmarkRecordStat` and
  `BenchmarkQueryLogPushDisabled` in the upstream and service packages.

### Changed
- **Resolver hot path.** Upstream stats are published through an `atomic.Pointer`
  so `recordStat` and `upstreamAvailable` no longer take the pool `RLock` on
  every answer; the cached latency order is an immutable snapshot read with a
  single atomic load (Hedged/Best Known). The query-log service caches its
  settings, so `Push` no longer reads (and decrypts) the setting from bbolt on
  every DNS query — the default, with logging off, is now 8.5 ns/op, 0 allocs.
- **Dashboard 1 Hz polish.** `renderCacheHealth`, `renderProxyDrops` and the
  latency badge compare before writing, and the resolver list and QPS chart are
  skipped while another tab is visible — the poll keeps running, only the paint
  is deferred.
- **Transport palette sharing.** The Android resolver chips resolve their colour
  and label from the theme's `ProtocolPalette` (`transportColor`/`transportLabelRes`)
  instead of a second local table, matching the live log and history cards.

### Fixed
- **Android detekt.** `TransportChips` no longer exceeds the cyclomatic-complexity
  threshold (the transport colour/label lookup moved to the theme).
- **Android alert channel name collapse.** The `weight(1f)` field now has a real
  row to fill instead of being squeezed beside the kind selector, switch and
  delete button.
- **Web alert type picker.** The native `<select>` (which cannot render icons)
  was replaced with a pictogram menu, fixing the icon-less kind dropdown.

### Notes
- No database schema migration is required; existing settings, clients,
  blocklists, certificates, relay agents and egress options are preserved.

## 🏷️ [v5.8.0] — query search and UI consistency

### Added
- **Query search.** An opt-in persistent query history, off by default and stored
  in a separate `querylog.db` beside the main database. Search by domain,
  client or account, action and protocol, with a 7-day default retention and
  anonymisation on by default. Available as a Tools card on the web panel and a
  Query Log screen on Android, with CSV and JSON export.
- **Query log API.** `GET /api/v3/query-log`, settings and export endpoints,
  documented in the v3 OpenAPI spec and mirrored on the web API.
- **Multi-select filters.** Actions and protocols are multi-select in both
  clients; protocols keep the app's colour in the menu and in the trigger.
- **Protocol palette.** One shared colour ramp for every surface: UDP cyan, TCP
  blue, DoT emerald, DoH purple, DoQ amber, DoH3 orange and DNSCrypt fuchsia,
  with a light-mode step. The live query log renders the transport as a tinted
  pill instead of bare text.
- **Copy feedback.** Copy actions confirm in place: the web panel swaps the
  glyph for a check, the portal tints the control, and Android flashes a green
  check for 1.2 s.

### Changed
- **Shared fields and buttons.** Login, change-password, diagnostics, add/edit
  client, bulk import, the API allow-list and the Domains/live-log toolbars now
  use `.panel-field` and `.panel-btn` instead of per-card utility strings.
  `.panel-field-sm` covers dense toolbars, and `.panel-field-end-icon` keeps an
  in-field glyph clear of the text.
- **One primary action per form.** Modal and full-width form commits use
  `.panel-btn-primary` (`.panel-btn-lg` when they own the card's width); the
  hand-written gradient buttons are gone.
- **Settings and phone navigation.** Settings is grouped under six captions
  above the sticky section jump bar. The phone bottom bar drops from ten targets
  to six (Dashboard, Domains, Filters, Users, Live Log, More); the rest live in
  a More sheet that reuses the same tab wiring.
- **Compact query filters.** Both clients show the first selected chip plus a
  `+N` count, so choosing more actions or protocols never grows the card or
  stretches the domain and client fields.
- **Android.** Query Log scrolls when the viewport is shorter than the content
  (landscape), while portrait still fits without page scrolling. Dialog corners
  align with card corners.

### Fixed
- **Query card dropdowns were clipped.** Paint containment on the card hid every
  entry below its bottom edge; the card now releases containment and the menus
  flip upward or downward based on viewport space and scroll when long.
- **The `+N` counter was clipped** by a wide first label; the chip now yields
  width so the number always shows.
- **Toggled buttons stayed visible** because `.hidden` lost to the component's
  own display rule; the two-class guard now matches `.badge.hidden`.

## 🏷️ [v5.7.0] — full-instance relay mode and unified egress cards

### Added
- **Full-instance relay mode.** A full DigitDNS instance can serve as a relay
  agent for another hub while continuing normal local operation. Hub traffic and
  local proxying share the same 80/443 listeners; hub clients are limited to the
  hub's approved host list, while local clients keep their existing behaviour.
- **Relay mode card** on web and Android: hub URL including the hidden admin
  path, write-only agent key, first-time enrolment token, online and sync state,
  hosts, clients, relay traffic, cached catalogue during hub outages, and a real
  **Test connection** that fetches the catalogue and reports ping.
- **Relay counters.** Relay traffic is counted separately from local proxy
  traffic for heartbeat and dashboard reporting.
- **API.** `GET/POST /api/v3/relay-mode` and `POST /api/v3/relay-mode/test`,
  mirrored on the web API and documented in the v3 OpenAPI spec.

### Changed
- **SOCKS5 and WireGuard cards now match relay-agent cards.** Status sits beside
  the card name; refresh, edit, enable/disable and delete are compact header
  icons; traffic and last-seen share one meta line; latency is a colour-coded
  ping badge; and **Test connection** sits in the footer. Cards refresh every
  ten seconds like agent cards.
- **Relay card layout.** Status sits beside the title, the agent number is pinned
  to the far right, and the ping badge has a fixed slot so the footer does not
  jump while testing. The separate refresh button was removed.
- **Android.** The engine ON/OFF control is shown only on the Dashboard, and the
  upstream **FASTEST** badge sits beside the hits/errors count.

## 🏷️ [v5.6.1] — engine switch and shell fixes

### Fixed
- **Android engine switch could not be turned back on.** `enabled: true` was
  omitted as a serialization default, so the enable request arrived as `{}` and
  was rejected; disabling sent `false` and worked.
- **Android engine switch colours.** The switch is green when on and red when
  off.
- **Folded web sidebar created a vertical scrollbar.** The sidebar is now a
  fixed shell: only the navigation list scrolls, while the bottom controls stay
  pinned.
- **Web System Log copy** is an icon inside the log window, markdown-style,
  instead of a header button.
- **Upstream Force button** sits closer to the interval chips.

## 🏷️ [v5.6.0] — egress options, engine switch and system log

### Added
- **Egress Options.** Relay Agents, SOCKS5 and WireGuard now share one page and
  one routing model. Relay agents remain public exit points; SOCKS5 and
  WireGuard are hub-side paths the relay dials through.
- **SOCKS5 egress.** Remote DNS by default, optional local resolution, optional
  username/password, health probe and traffic counters. Destinations still pass
  the hub's SSRF guard before dialling.
- **WireGuard egress.** Userspace `wireguard-go` + gVisor netstack, standard
  `wg-quick` configuration parsing, and `.conf` upload on web and Android.
- **Fail-closed egress.** An offline or disabled fail-closed egress answers
  SERVFAIL instead of falling back to this server. Fail-open egresses fall back
  to the direct path.
- **Grouped Route via.** This Server, Relay Agents, SOCKS5 and WireGuard groups;
  new values use `egress:<id>` and existing agent IDs are unchanged.
- **Global engine switch.** Engine OFF bypasses custom records, the rule
  matcher, proxying, service/agent assignments and ad blocking; queries resolve
  directly. Access control and quota remain enforced, and new SNI relays are
  refused while off.
- **System Log.** Bounded in-memory daemon log with a web page and Android
  Settings section, live refresh, copy and export/save.
- **By Provider ranking.** Top upstreams gains provider logos and a toggle that
  aggregates the ranking by provider.
- **Forced resolver selection.** Force bypasses adaptive hysteresis and selects
  the absolute fastest healthy resolver, marked with a FASTEST badge.
- **API.** `/api/v3/egresses`, `/api/v3/engine` and `/api/v3/system-log`.
- **Bring-your-own certificate.** The SSL / TLS Manager now accepts a PEM
  certificate and key, either uploaded/pasted or read from paths on the server,
  for the DoH/DoT listeners, the admin panel, and the subscriber portal. Pairs
  are validated for key match, hostname coverage, expiry and (optionally)
  self-signed private-CA use before they are written.
- **ACME DNS-01 wildcard issuance.** A Cloudflare API token and optional zone
  let the embedded ACME client issue and renew wildcard certificates such as
  `*.example.com`. The served hostname stays `dns.example.com`; only the
  certificate identifier is wildcard.
- **Certificate source record.** Each surface records whether it uses ACME or
  an operator-supplied pair, the ACME identifier, challenge type, paths and
  renewal policy. The DNS API token is stored encrypted and never returned by
  the API.
- **Startup and renewal honour the source.** A BYO DoH/DoT pair is loaded at
  boot and is not overwritten by the daily ACME loop; ACME wildcard renewals
  reuse the recorded identifier and DNS-01 settings.
- **API.** `GET/POST /api/v3/tls/cert` and `GET/POST /api/tls/cert` expose the
  source policy and install a pair. `/api/tls/issue` accepts `challenge`,
  `cert_name`, `dns_provider`, `dns_zone` and `dns_token`.

### Android
- **Certificate source card.** The SSL / TLS tab now offers Let's Encrypt
  HTTP-01, Let's Encrypt wildcard DNS-01, PEM paths on the server, and PEM
  upload/paste, for the panel, subscriber portal and DoH/DoT surfaces. Surface
  and Source are dropdowns, matching the web card instead of a scrolling chip
  row.
- **DNS-01 fields** for the wildcard certificate name, Cloudflare zone and API
  token.
- **PEM upload** with Android's document picker for the certificate and private
  key, plus an allow-self-signed switch for private CA/LAN use.
- **DoH3 switch** is now a real toggle; it is disabled while DoH is off.
- **Egress Options.** Relay Agents, SOCKS5 and WireGuard subcards, add/edit
  dialogs, `.conf` upload, health/traffic/fail-closed summary and a grouped
  relay picker.
- **Engine control** in the bottom bar / navigation rail.
- **System Log** section with copy and save.
- **Live Log filters** are coloured status chips instead of a popup.
- **Top upstreams** shows provider logos and a By Provider toggle.
- **Egress summary** stays on one row.

### Changed
- **Dashboard metric values** scale with the card's own width and no longer
  overflow when a card is resized.
- **Sidebar fold.** The desktop sidebar collapses to an icon rail and remembers
  the choice; engine and theme controls stay in the existing bottom rows.
- **Live Traffic** values use the same size as the other metric cards and align
  with them.
- **One transport policy.** The Upstream Resolvers transport chips and the
  Client Transports switches now write the same `DNSSettings` record. Disabling
  a chip stops the corresponding listener, and a listener switch updates the
  chips. DoH3 requires DoH, and an all-off chip selection keeps plain DNS on.
- The card is titled **SSL / TLS Manager for DoH, DoT**.
- The DoH/DoT and Subscription cards take the full settings width so the new
  certificate controls are readable.
- **Certificate upload pickers** are styled drop zones with the selected file
  name and a Browse action instead of native file inputs.
- **TLS card layout.** The DoH/DoT and Admin Panel certificate sections sit
  side by side as half-width panes on a wide panel, with compact inline
  **Issue certificate** buttons. Each pane states that the certificate is
  issued by Let's Encrypt, and the auto-renew labels mention Let's Encrypt.
- **Panel certificate auto-renew.** The Admin Panel pane now has its own
  **Auto-renew Let's Encrypt certificate** tick, backed by the same daily loop
  as the DoH/DoT switch.
- **Chromium flicker fixes.** The transport card no longer throws on an
  undefined draft field, DoH3 changes are included in the dirty check, hidden
  Domains rows are not rebuilt on background resolution polls, and the saved
  dashboard layout is applied before the grid is revealed.
- **Android certificate labels and validity.** The issue button is **Issue
  certificate** (or **Renew certificate**), the card title names Let's
  Encrypt, and the validity line is green with the number of days left. The
  DoT/DoH auto-renew card names Let's Encrypt.
- **Android Single Server state.** Disabled strategy, interval and transport
  chips keep readable muted colors instead of fading to black, and the
  interval footer shows **Last used: <resolver>** while Single Server is on.
- **Provider marks.** Control D / FreeDNS and the rest of the bundled resolver
  catalogue now map to their own logos on web and Android, instead of a generic
  globe.

---

## 🏷️ [v5.5.7] — adaptive upstreams and transport chips

### Added
- **Adaptive (fastest) strategy.** The pool is benchmarked every 5 minutes,
  15 minutes, 1 hour, 6 hours or 24 hours. The fastest healthy resolver is
  selected with 12% hysteresis, and the current resolver plus last-run details
  are persisted. **Force** runs the same check immediately.
- **Multi-select transport chips.** Plain, DoT, DoH, DoH3, DoQ and DNSCrypt can
  be enabled together; the resolver pool only uses the selected protocols.
- **Pool / Single Server switch.** Pool mode uses the configured strategy and
  transport chips. Single Server bypasses them, uses one resolver, and remembers
  the last selected server across Pool toggles.
- **Provider favicons** in the active resolver list and Single Server dropdown.
- **Current-resolver highlight**, using a neutral wash in the Graphite theme.
- **Forced adaptive-run API** at `/api/v3/settings/resolution/run-adaptive`.

### Changed
- **Retired strategy aliases.** “Pool default” is removed; an empty saved
  strategy now reads as Adaptive. “Fastest wins” is hidden and legacy `fastest`
  saves normalise to Hedged.
- **Reworked resolver card** in the web panel and Android app: strategy,
  interval and transport chips, abbreviated interval labels, one-row chip groups,
  two-line last-run text and the new switch row.
- **Android online status.** Returning from a long background period now reports
  **Online** while the SSE socket refreshes, instead of flashing **Connecting**.

### Fixed
- **Android background status.** A successful authenticated call during a stream
  retry now reports Online rather than Connecting.
- **Long single-server names** no longer collide with the next setting row.

### Android
- Version **5.5.7 (548)**.
- Biometric prompt is raised automatically when the lock screen opens.

### API
- Resolution settings now include `transports`, `adaptive_interval_minutes`,
  `adaptive_last_run`, `adaptive_current`, `adaptive_last_latency_ms` and
  `adaptive_last_note`.
- No database schema migration is required; existing records are normalised on
  load and save.

---

## 🏷️ [v5.5.6] — biometric login and a tighter dashboard

### Added
- **Android biometric login.** A switch in Settings → App, below the theme
  choice, locks the app behind fingerprint/face/device PIN. It prompts once to
  enable, locks on cold start and after a 60-second background grace period, and
  offers a sign-out fallback. Device credential is accepted, so a forgotten
  fingerprint is not a lockout.

### Changed
- **Uniform 12 px dashboard spacing.** Metrics, main and history grids and the
  section stack share one gap; the custom layout editor uses 12 px for both
  column and row gaps.
- **Compact stats polling.** The dashboard polls live fields every second and the
  full document every fifth poll, merging the slow CPU/RAM/system/agent fields.
- **In-place upstream rows.** The resolver card updates latency, health and
  tooltips without clearing and rebuilding the list.
- **Backend trims.** The blocklist index no longer re-normalizes an already
  normalized name; the agent list and Prometheus metrics are cached briefly.

### Android
- Version **5.5.6 (547)**.

---

## 🏷️ [v5.5.5] — faster resolver and dashboard paths

### Changed
- **Lock-free matcher reads.** The DNS query path loads an immutable rule
  snapshot instead of taking an RWMutex read lock on every query. Rule rebuilds
  publish a new snapshot atomically.
- **Client list from the in-memory index.** `ListClients` no longer decrypts
  every record from bbolt on each dashboard poll; it snapshots the service's
  existing index and adds the live traffic/query deltas.
- **Agent catalogue body cache.** Encoded catalogues are cached per agent and
  control-plane revision, so a fleet does not repeat the assignment walk and
  JSON encode for the same revision.
- **Gzip writer pooling** for API responses, and a one-second cache for the v3
  upstream-stats endpoint.
- The quota-refusal log reuses the already-parsed DNS question.
- Login page branding now reads **SmartDNS · SNI Proxy · Ad Blocking**.

### Android
- Version **5.5.5 (546)**. No functional Android changes; the backend changes
  are transparent to the existing v3 API.

---

## 🏷️ [v5.5.4] — setup checklist, change previews and working API docs

### Added
- **First-run checklist on the dashboard.** Six live steps — admin password,
  2FA, panel TLS, DNS listener, upstreams and the first client — derived from the
  running configuration rather than a stored "done" flag.
- **Change preview before saving.** Routing rules and ad-blocking settings now
  show a redacted before/after diff and require confirmation before the write.

### Fixed
- **API v3 documentation opens from the dashboard.** The docs page accepts the
  dashboard document cookie, so it works with a loopback-only `api_bind`. The
  spec is inlined and the Swagger bootstrap moved to an external file that the
  CSP allows.
- **Server Update card stays expanded on Docker.** The GitHub check no longer
  inherits the browser request cancellation, results are cached for a minute,
  and a failed check still carries the container fields.
- **2FA enrollment can be cancelled**, so an abandoned enrollment no longer
  leaves the settings card expanded after a reload.
- **Ad Blocking Rule Tester** moved directly below the settings card at half
  width on desktop and stacked on mobile.

### Changed
- API v3 spec expanded to cover every router route, with descriptions, request
  bodies, examples and a drift test.
- Splash, dashboard footer, About card, subscriber portal and systemd unit now
  use the complete SmartDNS, SNI-proxy and ad-blocking wording.
- Visible documentation no longer advertises old v2.x version headings.

---

## 🏷️ [v5.5.3] — quieter panel and faster agent recovery

### Changed
- **Removed the global upstream-degraded banner.** A single failed DoH3 probe
  could push a page-wide warning across every tab. Per-resolver health remains
  on the Upstream Resolvers card (dot, latency and tooltip); the listener
  warning for a transport that is enabled but not listening stays.
- **Agents recover from a long hub outage faster.** Heartbeat backoff is capped
  at one minute instead of sharing the catalogue’s five-minute ceiling, so an
  agent reappears promptly when the hub returns.
- **HTTP/2 keepalive and GOAWAY handling** for the agent-hub client: idle
  connections are pinged, and a GOAWAY re-arms the long-poll immediately.
- **Android live-query reconnect is event-driven.** The 500 ms flag-polling loop
  was replaced with a completion signal from the stream close/reconnect paths.

### Removed
- The unused **2.8 MB** legacy Tailwind stylesheet is gone from the source tree
  and Docker context.
- Visible documentation no longer advertises old v2.x version headings.

---

## 🏷️ [v5.5.2] — trustworthy upstream latency

### Added
- **Adding a resolver benchmarks it automatically.** The web and Android add
  endpoints run a single-resolver probe asynchronously, so a new upstream gets a
  latency on the next stats poll instead of staying “not measured”.
- **`latency_is_probe` in the upstream stats API**, so clients can distinguish a
  warm benchmark median from a live-query median.

### Fixed
- **Benchmarks warm up first and report the median of three measured queries.**
  The discarded warm-up absorbs the DoT/DoH/DoQ/DoH3 handshake, which previously
  made a freshly benchmarked resolver look much slower than the same resolver on
  a re-run. A benchmark requires a majority of samples to answer.
- **A single slow live query no longer dominates the latency card.** Live p50 is
  used only after three successful live exchanges; until then the card shows the
  stable benchmark median and labels it as a probe value.
- **Web provider rows toggle cleanly.** Clicking a checked server removes it
  from the card immediately, with an optimistic pending state and a short
  mutation guard against stale stats polls.
- **Web upstream field no longer triggers address autofill**, the dropdown uses
  larger type, and the upstream card title is aligned with the chart card.
- **Typing a bare provider name** such as `quad9` now asks the operator to choose
  one of its servers instead of creating an invalid `quad9:53` host.

---

## 🏷️ [v5.5.1] — DoQ completion and installer sync

### Fixed
- **DoQ queries now close the QUIC write side (STREAM FIN) after the single
  query, as RFC 9250 §4.2 requires.** Strict public DoQ resolvers (AdGuard,
  Quad9 and others) were waiting for the rest of the message, so the upstream
  showed no latency and could be treated as unmeasured. A regression test now
  waits for FIN before replying.
- **The offline-bundle installer opens DNSCrypt TCP/UDP 5443** in its firewall
  rules, matching the online installer. The CI installer-sync check covers this
  again.

### Operations
- Server-only fix; the Android app is version **5.5.1 (542)** for release
  packaging and has the same behaviour as 5.5.0.

---

## 🏷️ [v5.5.0] — client transport authentication, DNSCrypt and mTLS

### Added
- **DNSCrypt server** on UDP and TCP **5443**, with encrypted provider keys stored
  in their own setting, a signed certificate, a copyable stamp and live listener
  status. The default port avoids TCP 443, which the SNI proxy owns. Enable it in
  **Settings → Client Transports**; the Android Transports screen has
  the same controls.
- **DoH bearer tokens:** `/dns-query` accepts `Authorization: Bearer <token>` as
  well as the historical `?token=` parameter, so the credential stays out of
  access logs and browser history.
- **DoT SNI-token authentication:** the ServerName becomes
  `<token>.<token domain>`, checked before any query is read. DoT keeps its own
  token list, independent of DoQ's.
- **mTLS** for DoT, DoH, DoH3 and DoQ from an operator-supplied client CA bundle.
  DoT requires a verified certificate at the TLS handshake; DoH and DoQ request
  one and enforce verification on the DNS query path so the subscriber portal on
  the DoH port stays reachable.
- **Plain DNS and DoH switches:** plain UDP+TCP and the DoH endpoint can be
  disabled independently, and both expose their port. The DoH switch gates only
  `/dns-query`; the subscriber portal keeps serving on that port.
- **A complete Client Transports card:** Plain DNS, DoT, DoH, DoQ, DoH3,
  DNSCrypt and the mTLS policy tile in one place, each with a port, a copyable
  endpoint and the switch in the same position. DoH3 shows the DoH port as
  read-only because they share it over UDP.
- **Public DNS provider browser:** the upstream card's add box now searches a
  curated top-ten provider catalogue (Cloudflare, Google, Quad9, OpenDNS,
  AdGuard, Control D, DNS.SB, Yandex, NextDNS and CleanBrowsing), grouped by
  provider with bundled logos and every supported transport. A `+` adds one
  endpoint; typing a custom address still works. The same catalogue is served
  to Android.
- **Prometheus:** `digitdns_dnscrypt_queries_total` and
  `digitdns_dnscrypt_errors_total`.

### Changed
- `/api/settings/transports` reads and writes the DNSCrypt, DoT-authentication
  and mTLS fields, plus `dot_enabled`. A failed apply still rolls the listeners
  and the generated DNSCrypt keys back.
- **DoT has an enable switch** and defaults to on, so existing installs keep
  serving port 853 after an upgrade.
- **Transport edits autosave:** switches and completed fields apply as they are
  edited, and the global Save button is gone on web and Android. Incomplete
  SNI-token or mTLS forms stay local until their required inputs are present.
- **The web card is named Client Transports**; Android keeps its Transports
  label. The startup summary now lists every transport, including disabled
  ones, and DNSCrypt and mTLS.
- **Endpoints tell the truth about their certificates:** Plain DNS and the
  DNSCrypt stamp use the public IP, while DoT, DoH, DoH3 and DoQ use the
  DoH/DoT certificate domain (SNI-token modes still use the token hostname).
  The card's top-right badge names every transport that is not on.
- **DoH3 is always on** and shares the DoH port; its switch is read-only on web
  and Android.
- **DNSSEC saves apply live.** The validator is swapped in the running handler
  and the cache is flushed, so the RUNNING badge changes in the same request.
  The save button is now **Save & Apply DNSSEC Settings**.
- **Android DNSSEC/Proxy Leak first paint** no longer runs the pull-to-refresh
  arc during the initial request, and the cards keep their layout while the
  values load.
- Changing DoT authentication or the mTLS gate rebinds the TLS listeners; an
  unrelated socket change does not. A rebind closes the old listening socket
  before binding the new one.
- DoT and DoH now capture their server in the serve goroutine, removing a
  nil-dereference race during a live rebind.

### Fixed
- **Web Client Transports no longer flickers while a switch is saved.** Card
  status labels, endpoints, the DNSCrypt stamp, token lists and the status badge
  are updated only when their value changes; the old `Saving…` rewrite made all
  six cards repaint on every autosave. The badge comparison uses `textContent`,
  because CSS uppercasing made every `innerText` comparison look different.
- **Android transport switches stay in place.** The save request now emits every
  transport field, so a switch can no longer bounce back because an omitted
  default was read as "leave the old value". Status badges and switches share a
  fixed, right-aligned slot that does not move when the status changes.
- **Android settings pages no longer open behind a full-screen spinner.** Card
  skeletons fill the first paint on Settings, Security, REST API, Subscription,
  Client Transports, Diagnostics, IPv6, DNSSEC/Proxy Leak, History, Relay Agents,
  Access, Upstreams, Ad Blocking, Groups, Policies and Connect; initial loads no
  longer run the pull-to-refresh arc.
- **The Android RAM card leads with the instance's own allocation**, with
  machine-wide server RAM and usage moved to the secondary lines. The web RAM
  card follows the same order.
- **Android relay selection uses the full dialog width**, with larger country
  flags, relay names and detail text. Service and relay pickers now use the same
  check-circle/radio pictograms as the provider browser, and a selected relay row
  is tinted.

---


## 🏷️ [v5.4.3] — faster dashboard and clearer state

### Performance
- **API gzip:** authenticated JSON/text responses are compressed when the client
  accepts it. `/api/filtering` ~22 KB → ~5.3 KB, `/api/metrics` ~7.7 KB →
  ~1.7 KB, `/api/config` ~2.2 KB → ~0.9 KB. Bodies under 512 bytes remain
  identity-encoded.
- **Minified assets:** `app.js` 551 KB → 298 KB raw (~153 KB → ~80 KB gzip) and
  `style.css` 134 KB → 58 KB raw (~39 KB → ~11.8 KB gzip), served under the
  existing URLs. A source-hash test prevents stale minified output.
- **Chart.js deferred:** no longer loaded on `DOMContentLoaded`; it loads after
  the first stats poll, and not on the login page.
- **Less per-poll layout work:** `textContent` replaces `innerText` for stats and
  latency updates; the QPS chart repaints at most once per ~1.5 seconds.

### Added
- **Global listener notice:** DoH3/DoQ enabled but not listening is shown on
  every tab with the server's `last_error` and an **Open transports** action.
- **Global upstream notice:** degraded resolvers are named with a
  **Review upstreams** action, using the monitor's health predicate.
- **“Why was this blocked?”** Blocked query-log rows show `Blocked by …` with
  the custom rule or blocklist source and the matched pattern, including on
  phones where the RULE column is hidden.

### Fixed
- `/api/config/access` now reports the live access mode instead of a stale
  `access.allow_all` copy, so the dashboard and resolver can no longer disagree
  about whether public mode is on.
- Removed the full-width public-mode banner; the Clients page owns that switch.

### Operations
- No resolver, API or database schema migration. Existing settings, clients,
  blocklists and certificates are preserved.
- Android carries only the **5.4.3 (540)** version bump; no Android behaviour
  changes.

## 🏷️ [v5.4.2] — compact Android Transports screen

### Changed
- Android **Settings → Transports** now uses an adaptive layout:
  - on panels 700 dp and wider, DoH3 and DoQ sit side by side;
  - on phones the cards stack, with the intro card replaced by one muted line,
    the listener status moved into each card header, and the authentication
    label removed.
- The DoQ IP-allowlist summary, counts and Public Mode warning are combined into
  one compact note.
- The SNI-token list is capped at 88 dp and scrolls internally, so a large token
  set no longer stretches the card.
- The outer scroll remains only as a safety net for very short screens or the
  SNI-token editor. The standard IP-auth screen fits without scrolling on a
  411 × 914 dp phone and a 1280 × 720 dp tablet.

### Fixed
- Removed four unused Android strings left over from the old Transports layout.

### Operations
- Android-only UI patch. Resolver, API, database and server transport behaviour
  are unchanged. No migration is required.

## 🏷️ [v5.4.1] — transport hardening and allowlist clarity

### Added
- **DoQ IP-allowlist clarity** on web and Android: the card names its sources
  (**Clients → Allowed IPs** and **Settings → Access → Server-wide Resolver
  Allowlist**), shows live client/entry/range counts, warns when Public Mode
  (`allow_all`) makes the list irrelevant, and warns when whitelist mode has no
  entries.
- **QUIC handshake rate limiting**: 60 burst / 2 per second per source IP,
  checked before TLS on DoH3 and DoQ, with a bounded source map.
- **DoH3 connection cap** of 1,024 live connections, matching DoQ.
- **Prometheus QUIC series**: `digitdns_doh3_connections`,
  `digitdns_doh3_connections_total`, `digitdns_doq_connections`,
  `digitdns_doq_connections_total`, `digitdns_doq_protocol_errors_total` and
  `digitdns_quic_handshake_rejected_total{transport=...}`.
- **Periodic upstream health check**: every five minutes the resolver re-probes
  each upstream and logs one warning when an upstream is degraded, plus one
  recovery line. Degraded means an open circuit, a failed latest probe, or a
  recent live error rate of at least 10% over 20 queries.

### Changed
- **DoH3 applies immediately** from its switch. **DoQ is staged** behind
  **Save DoQ** because its port, authentication mode, token domain and token
  list must be valid together.
- Transport saves are now **applied before they are persisted**. A failed bind
  rolls the listeners back to the previous state instead of leaving a saved
  switch ahead of a dead listener.
- The transport card keeps the failure reason in `last_error`, including a
  listener that fails after the save returns.
- Android Transports matches the hybrid behaviour and shows applied versus
  staged DoQ state.
- The Android **Save DoQ** button now shares a row with the firewall note.

### Fixed
- DoH3 reported “listening” as soon as the UDP socket was bound. It now waits
  for the HTTP/3 serve loop, and a listener that dies later is reported as
  stopped instead of looking healthy.
- A failed DoH3/DoQ apply no longer leaves the saved switch enabled while the
  listener is down.
- Disabling a transport clears a stale boot-time listener error from the card.
- The DoH3 card shows the configured DoH port instead of a hardcoded 8443.

### Operations
- No database migration. Existing DoH3/DoQ settings and DoQ tokens survive the
  upgrade.
- The default DoQ port remains 853/UDP; DoH3 shares the DoH port on UDP.
- `9.9.9.9` was removed from the Norwich deployment after repeated upstream
  timeouts; the release code itself does not change the configured pool.

## 🏷️ [v5.4.0] — client-facing DoH3 and DoQ

### Added
- **DoH3 (HTTP/3)** as an opt-in client transport on **UDP 8443**, using the
  same `/dns-query` URL and certificate as DoH. TCP DoH advertises the endpoint
  with `Alt-Svc` while it is listening; incapable clients fall back to TCP DoH.
- **DoQ (DNS-over-QUIC)** as an opt-in client transport on **UDP 853**,
  implementing RFC 9250 framing, transaction ID 0, one query per stream,
  connection/stream caps and no 0-RTT.
- **DoQ authentication** in two modes: the existing client IP allowlist, or
  SNI-label tokens (`quic://<token>.doq.example.com:853`). Token mode requires a
  certificate covering the token hostnames; the built-in ACME client cannot
  issue wildcards.
- **Settings → Client Encrypted Transports**: a full-width, two-column card
  with DoH3/DoQ switches, live listener status, copyable endpoints, DoQ port,
  auth mode, token domain and token list, and one Save action.
- **Connect Guide and subscriber portal** endpoint cards for DoH3/DoQ.
- **Android Transports settings screen** with the same DoH3/DoQ switches, port,
  auth mode and token list; Android Connect and subscriber guides show the new
  endpoints when enabled.
- **Prometheus** counters `digitdns_doh3_requests_total`,
  `digitdns_doq_queries_total` and `digitdns_doq_auth_failures_total`.
- `GET/POST /api/settings/transports` and its v3 equivalent; `GET /api/config`
  now includes `doq_url` and the transport view.

### Changed
- The installer banner, firewall guidance and Docker `EXPOSE` list mention
  **UDP 853** and **UDP 8443**. Both transports remain disabled by default.
- The SNI proxy cards report **actionable refusals** and **unreadable scanner
  traffic** separately, on web and Android, instead of adding them into one
  “drops” figure.
- The DoQ token list is seeded before the listener starts, so an install that
  boots with DoQ already enabled enforces its tokens immediately.

### Fixed
- A DoH3/DoQ bind failure is reported to the caller and leaves the listener
  off; the card shows the actual listener state rather than the saved switch.

### Operations
- Open **UDP 853** and **UDP 8443** in the firewall.
- DoH3 needs the normal DoH/DoT certificate. DoQ SNI-token mode needs a
  wildcard or multi-SAN certificate; HTTP-01 cannot issue wildcards.
- DoQ is not used by Android Private DNS; that remains DoT.

## 🏷️ [v5.3.3] — accurate SNI proxy drop counters

- The web and Android SNI proxy cards no longer count flood attempts from a
  source already in the malformed-probe penalty box as relay drops.
- The card headline now reports actionable refusals plus unreadable
  connections; the full breakdown, including excluded flood and malformed
  probes, stays in the web tooltip.
- Prometheus adds `digitdns_relays_refused_actionable_total` and
  `digitdns_relays_flood_total`; `digitdns_relays_refused_total` keeps its
  existing total semantics.
- No resolver, filtering, routing or database behaviour changes.

## 🏷️ [v5.3.2] — balanced Settings layout

- Server Location grows to fill the left column, so it ends level with Server
  Update on both systemd and Docker; its title shares the update card's padding
  and top alignment.
- The container image/binary selector only changes the update card's height;
  the Location card absorbs the difference, leaving no gap before Hidden Admin
  Path and no stretched card on systemd.
- On panels 1400 px and wider, Subscription Portal and the SSL / TLS Manager
  share a row. The TLS card stretches to the taller card's height, and its field
  rows reflow to two columns with full-width action buttons.
- Below 1400 px both cards keep the full-width stacked layout; mobile and narrow
  windows are unchanged.

## 🏷️ [v5.3.1] — compact Settings top row

- Server Location is now a one-line card: country and the GeoIP database date
  share a row, and the card is roughly half its previous height.
- Two-Factor Authentication sits directly underneath it in a tight left column;
  Server Update occupies the right column. The old full-width 2FA row is gone.
- The layout is identical on systemd and Docker. The container image/binary
  selector only makes the update card taller; it no longer decides where the
  2FA card can go.
- The GeoIP database age remains available as the inline `DB <date>` text and as
  a tooltip.

## 🏷️ [v5.3.0] — faster cached queries, metrics docs and Android startup

### Performance
- The cached query path skips `applyQueryOptions`/DNSSEC stripping when the entry
  was stored already normalised for the same EDNS/DO/CD key. `MatchDetailLower`
  and `LookupWithOptionsLower` reuse the lowercase name the resolver already
  computed. `BenchmarkProcessCacheHit`: ~932 ns → ~770 ns.
- Agent fleet counts in `/api/metrics` are cached for five seconds.
- Dashboard favicons moved out of `index.html` into `/img/favicon-*.png`
  (document 228.6 KB → 219.8 KB) and are served from the immutable asset URLs.

### Monitoring and docs
- New `docs/METRICS.md` and `docs/grafana/digitdns.json`.
- `docs/API.md` lists `/api/metrics`, `/api/update/mode`, `/api/filter/test`
  and their v3 equivalents.
- Relay Agents on web and Android show the applied catalogue revision and age.
- The update card links to release notes and can copy the compose command; server
  notes are shorter.

### Android
- `NavigationRail` replaces the bottom bar at ≥600 dp.
- Quick Settings tile toggles ad blocking through the v3 filtering API.
- Baseline and startup profiles generated with `:baselineprofile` and packaged
  via `androidx.profileinstaller` (`assets/dexopt/baseline.prof`).

### CI
- cosign is pinned to v2.5.2 and downloaded with retries instead of the floating
  installer action that could time out and redden `Docker main`.

## 🏷️ [v5.2.13] — agent control plane without the restart stampede

- The agent control-plane revision is persisted (`agent_revision`) and the
  catalogue ETag now carries the build fingerprint: `"v<counter>-<hash>"`. A hub
  restart with unchanged config answers 304 instead of forcing every agent to
  pull a full catalogue; an upgrade still invalidates because the hash changes.
- The two global catalogue inputs (rules and the panel client list) are cached
  per revision. A config change now costs two database reads in total, not two
  per agent.
- Heartbeats use `bolt.Batch`, so concurrent beats share one transaction and one
  fsync. `TouchAgent` returns the record it wrote, removing the extra `GetAgent`
  the handler used for the live router.
- The agent's five-minute safety full sync moved to 30 minutes. Changes still
  propagate immediately through the long-poll; the heartbeat revision check is
  the fallback.
- The agent's hub client uses an explicit transport: four idle connections per
  host, 90 s idle timeout, 5 s TLS handshake and HTTP/2 when the hub supports it.
- A transport-level end to an established long-poll (hub restart, idle proxy) is
  no longer treated like a rejection: the next successful heartbeat re-arms the
  poll immediately. HTTP errors and 5xx keep the exponential backoff.
- New Prometheus series: `digitdns_agent_catalogue_requests_total`,
  `_not_modified_total`, `_builds_total`, `_long_polls`, `digitdns_agent_revision`,
  `digitdns_agents_online` and `digitdns_agents_total`.

## 🏷️ [v5.2.12] — container update method is now a choice

- Docker installs get **Settings → Server Update → Container update method**:
  - **Pull image (recommended)** keeps the previous behaviour: `supported:false`,
    no in-app swap, and the compose pull/recreate command.
  - **Patch binary in this container (temporary)** re-enables the in-container
    swap. Signature verification, checksum, backup and the restart watchdog are
    unchanged. The image is not updated.
- The choice is stored in its own encrypted `update` settings record and applied
  at startup. It is container-only: on systemd the selector is hidden, the API
  rejects a change, and the updater ignores any stale value.
- A marker beside the backups records the container ID, the image version and
  the swapped version. The panel shows **“Temporary binary update active — a
  container recreate restores vX”** while the swapped binary runs, and clears it
  when a recreate changes the container ID.
- `POST /api/update/mode` (web session) and `POST /api/v3/update/mode` (master
  key) set the method.
- Android: the About page shows the same selector (admin-wide, container-only),
  the temporary badge, and the update card first. The page was compacted so the
  update controls fit without scrolling on a phone; the informational cards
  follow.
- No database migration. No resolver, relay or agent behaviour changes.

## 🏷️ [v5.2.11] — faster panel, Prometheus metrics and a rule tester

- Asset URLs now carry a content fingerprint (`/js/app.js?v=…`). A matching
  request is served with `Cache-Control: public, max-age=31536000, immutable`;
  the document and stale fingerprints still revalidate, so an upgrade is never
  pinned to an old bundle.
- The dashboard polls `/api/stats` once a second only on the Dashboard tab and
  every five seconds elsewhere, refreshing immediately when the tab returns.
- `script-src` no longer allows `'unsafe-inline'`. The two dashboard inline
  scripts and the per-admin-path login script are authorised by SHA-256 hashes.
- New `GET /api/metrics` (Prometheus text format, guarded by the master REST
  key or a dashboard session): build info, queries, QPS, cache, refreshes,
  relays, CPU/RAM, goroutines and latency quantiles. No unbounded labels.
- New read-only rule tester: `GET /api/filter/test?domain=…` and
  `GET /api/v3/filter/test`, with a card in the web Ad Blocking tab and the
  Android Ad Blocking screen. It reports the action, the matching list and
  whether an exact entry or a parent wildcard applied.
- The aggregator normalises the fixed action/protocol vocabulary without
  allocating on the common path; `BenchmarkBucketRecord` records the result.
- Android: ordinary API calls have a 20-second read timeout again. Only the SSE
  client disables it, so a stalled stats call can no longer hang a coroutine.
- Android: the upstream resolver rows no longer show the amber
  `transport … · policy … · last: …` line. The status dot and latency colour
  remain.

## 🏷️ [v5.2.10] — update modal can no longer spin forever

- `GET /api/update/status` now clears a `Running` flag that has outlived the
  apply deadline (six minutes) and reports an actionable error. Previously the
  progress modal polled that stale state indefinitely; only a further click on
  Update was forgiven.
- The progress modal treats `running: false` as a terminal state. When the
  daemon restarts faster than a poll observes `restarting`, the in-memory
  progress is already idle; the modal now reloads instead of polling an empty
  status every 1.5 s forever.
- Container installs continue to refuse the binary-swap path and show
  `docker compose pull && docker compose up -d` (v5.2.9 behaviour).

## 🏷️ [v5.2.9] — Docker-aware updater and no stuck apply lock

- The self-updater now detects Docker/Podman (`/.dockerenv`, `/run/.containerenv`,
  the container cgroup, or `DIGITDNS_CONTAINER=1`). Container installs report
  `supported: false` and show the correct command:
  `docker compose pull && docker compose up -d`.
- Container `POST /api/update/apply` is refused with that command. The old path
  swapped `/app/digitdns` inside the container's writable layer; it never pulled
  the image, and the next recreate reverted it.
- A `Running` apply older than six minutes is treated as stale and a new attempt
  is allowed. A restart signal that fails, or a process that does not exit within
  15 seconds, now sets an error and releases the lock instead of leaving every
  later click on “an update is already in progress”.
- systemd installs keep the verified binary swap, backup and restart path.
- The update card displays the server's note, so the pull/recreate instruction is
  visible where the operator checks for updates.

## 🏷️ [v5.2.8] — web live log survives buffered SSE

- Added an authenticated `/api/stream/recent` endpoint that returns the server's
  recent-query ring. The web log falls back to it when the EventSource opens but
  delivers no frames for three seconds, which is what a proxy that buffers SSE
  looks like. Polling stops as soon as an SSE event arrives; it polls only while
  the Log tab is visible and does nothing when the ring has not changed.
- The empty state now says whether the log is connecting, connected via polling,
  listening for queries, or paused, instead of always saying “Listening”.
- Pause/Resume has a fixed 96 px box, so the search bar and filter row no longer
  resize when the label switches between the two verbs.
- The filter compile line now reports `rules (exact + suffix) / unique domains`,
  making it clear why a plain-domain list stores roughly two lookup entries per
  name (exact for the apex, suffix for subdomains).

## 🏷️ [v5.2.7] — web live log stays connected

- The browser live-log stream is opened once at dashboard boot and kept alive
  while the page is visible, including while the operator browses in another tab
  or another panel screen. The earlier per-tab gating dropped queries whenever
  the Log tab was not focused.
- Query rows are still buffered while the Log tab is hidden; returning to it
  repaints from the buffer without a reconnect.
- A 401 from the stats poll or the SSE ticket now raises the login prompt instead
  of leaving stale tiles and a dead stream after a daemon restart wipes the
  in-memory web session.
- Stream startup is single-flight and generation-guarded, so a visibility change,
  refresh or sign-out cannot leave two EventSources or an orphaned stream.

## 🏷️ [v5.2.6] — reliability boundaries and responsive agent control

- **Panics are contained at request boundaries.** The DNS handler, SNI and agent
  connection handlers, and upstream exchanges recover, log the stack and count
  `panics_total` instead of terminating the daemon. The DNS handler still answers
  SERVFAIL for the affected query.
- **The circuit breaker has a half-open state.** After the cooldown, exactly one
  probe is allowed; concurrent queries keep skipping the resolver until that probe
  reports. Recovery is jittered and exposed as `circuit_state`
  (`closed`/`open`/`half-open`).
- **Agent heartbeat and catalogue sync are jittered and backed off.** A hub restart
  no longer makes every agent reconnect on the same tick.
- **Cached JSON is gzip-negotiated.** `/api/stats`, `/api/v3/stats` and the combined
  history document compress once per cache fill; the dashboard stats payload drops
  from ~6 KB to under 1 KB on the wire.
- **The live query stream is opened only while the Live Log tab is visible.** Home
  uses the 1 Hz stats poll and no longer parses every query event.
- **Malformed-probe penalty box.** A source that produces repeated malformed
  handshakes is dropped for a minute before a handshake read or relay slot is spent;
  the drops appear as `relays_refused_flood`.
- **Hub↔agent control plane.** Catalogue responses carry an `ETag`; unchanged
  allowlists answer `304` without recomputing hosts or clients, and the payload is
  gzip-negotiated. Agents long-poll the catalogue with a 25-second wait, so a rule
  or client change reaches them in under a second instead of at the next 60-second
  sync, with a five-minute full fetch as a safety net.
- **Agent liveness and sync state.** The online window is 90 seconds (was two
  minutes), computed server-side and reused by the dashboard. Agent heartbeats
  carry the applied catalogue revision, the panel shows its age, and the agents
  tab refreshes every 10 seconds while visible.
- **Agent relay path.** Target hosts use a bounded 60-second DNS cache, multiple
  addresses are dialled with a 200 ms stagger instead of a 10-second wait on the
  first, both sockets get `TCP_NODELAY`/keepalive, and the listener has a 2,048
  connection cap, a 5-second handshake timeout and the malformed-probe penalty box.
- **Faster agent failover.** Proxied answers now use a 30-second TTL (the hub's own
  egress keeps 60), so a dead agent leaves client caches sooner.

## 🏷️ [v5.2.5] — trustworthy upstream metrics, faster relay and stats

**Upstream metrics.** The resolver now records the transport round trip returned
by the exchange, not the wall-clock time of the goroutine around it. Latency is
the median of the last 128 successful live exchanges (`p95` in the tooltip)
instead of an EWMA that one slow DNSSEC or TCP-fallback query could inflate for
minutes. The seeded 12 ms estimate is gone: an upstream that has not answered
reports `measured: false` and renders `—`.

- Startup probes are counted separately (`probe_hits`, `probe_errors`,
  `last_probe_ms`) and no longer inflate live hits/errors or trip the breaker.
- `recent_error_rate` over the last 64 live outcomes drives the health dot, so a
  timeout an hour ago does not keep a working resolver amber.
- The dot means health; the latency badge keeps its own colour. A slow but
  healthy resolver stays green.
- Policy rcodes (REFUSED/SERVFAIL) count as reachable and stay distinct from
  transport failures.

**Performance.** The SNI relay uses `TCPConn.ReadFrom` (splice) for unmetered
relays while preserving the idle timeout; metered relays keep the buffered,
per-1 MB accounting path. The relay buffer is now 64 KiB. Live stats and the
combined history endpoint are cached per process (1 s and 5 s respectively), so
several dashboards share one computation.

## 🏷️ [v5.2.4] — SNI Refused counts real refusals only

**A patch for the SNI proxy card.** Public port 443 constantly receives
connections with invalid SNI/Host values. Those malformed probes were counted in
`relays_refused`, so a healthy proxy looked like it was refusing traffic all day.

- Malformed SNI/Host probes no longer increment `relays_refused`. They are
  counted separately as `relays_refused_malformed` and shown in the Refused
  tooltip as “malformed probes … (not counted)”.
- `relays_refused` now covers only actionable refusals: blocked targets,
  self-dials, spent quota, a full relay table and resolution failures.
- The card stays quiet on a public proxy port; no extra line was added.
- No database migration and no other behavior changes.

## 🏷️ [v5.2.3] — Faster resolution and clearer diagnostics

**A performance and operations release.** The resolver defaults to Hedged
upstream racing, reuses DoT/DoQ connections, and loads history and settings with
far fewer requests. The SNI proxy and upstream cards now explain why
connections or answers fail, and a circuit breaker stops repeatedly paying for a
dead upstream.

### ⚡ Resolver and upstream performance

- Default racing strategy is **Hedged**; Fastest remains selectable. A cache
  miss no longer fans out to every upstream by default.
- Persistent **DoT connection pool** and **DoQ connection reuse**; a stale
  connection is retried once instead of surfacing as a query failure.
- TLS 1.3 session cache shared by DoT/DoH/DoQ; DoH idle/max connections tuned.
- Immutable query-path snapshot, pre-filtered transport candidates and a cached
  latency order, removing per-miss slice copies, locks and map/sort work.
- Relay targets carry pre-parsed IPs; the rate limiter uses allocation-free
  `netip.ParseAddr`.
- Tests pin five DoT queries to one TLS connection and five DoQ queries to one
  QUIC connection.

### 📊 Faster panel and Android

- New `GET /api/v3/stats/history` returns the summary, series and all six ranked
  tables in one response. Web and Android history moved from **8 requests to 1**.
- `/api/v3/stats` now carries the upstream rows; Android no longer makes a
  second three-second poll for them.
- Web boot defers Agents/DNSSEC/Proxy Leak loads until their tab opens.
- Chart.js is loaded on demand from the admin namespace, so the first screen
  paints before the chart library.

### 🩺 Diagnostics

- SNI proxy refusal breakdown: malformed, blocked, self-dial, quota,
  table-full and resolution failure. The reasons are exposed in `/api/stats`
  and the Refused tooltip; malformed scanner probes no longer look like relay
  failures.
- Upstream errors are split into **transport failures** and **policy rcodes**
  (`REFUSED`, `SERVFAIL`, etc.), with `last_error`, `last_error_at`,
  `consecutive_failures` and `circuit_open` in the live stats.
- **Circuit breaker**: opens after five consecutive transport failures, stays
  open for 30 seconds, is reset by a success or a policy rcode, and fails open
  when every candidate is open.
- The web and Android show the state compactly: a red/amber dot and a detailed
  tooltip, with no extra card line or chip.

### ⚙️ Upgrade notes

- No database migration.
- An empty/default strategy now means Hedged. Explicit Fastest, In Order or Best
  Known selections are unchanged.
- The circuit breaker is automatic and has no setting.

## 🏷️ [v5.2.2] — Optional IPv6 for the hub and relay agents

**A feature patch on the 5.2 line.** IPv6 for proxied names is now optional and
controlled end to end: a hub-wide master switch plus a per-agent opt-in. The
same release removes the transient “session expired” flash at sign-in and makes
the Android Relay Agents and Traffic History cards fit a phone screen.

### 🌐 Optional IPv6

- New server-wide `proxy_ipv6_enabled` master switch and `public_ipv6` address.
  Proxied AAAA is published only when both are set; direct names keep their
  normal AAAA.
- New per-agent `ipv6_enabled` opt-in and `public_ipv6` address. Effective
  policy is the per-agent opt-in **and** the global switch.
- The hub delivers the effective policy in the agent catalogue and heartbeat
  reply. An agent applies a change on its next heartbeat, within 30 seconds.
- Turning IPv6 off makes an agent stop IPv6 detection and reporting and rebind
  IPv4-only (`tcp4`). It returns to dual-stack when both switches are on again.
- Proxied AAAA is withheld while IPv6 is off. Proxied `HTTPS`/`SVCB` hints stay
  sunk even if proxy leak protection is disabled.
- Added server-settings, agent, DNS, relay-router and web tests.

### 🖥️ Web panel

- New **Settings → Proxy IPv6** card with the live switch, public IPv6 field,
  leak warning and saved state.
- Relay-agent form gains **Public IPv6** and **Use IPv6 for this agent**.
- Agent cards show the reported IPv6 address and the effective IPv6 state.
- Sign-in no longer flashes “session expired”: a 401 re-raises the login gate
  silently, the history panel stops writing the auth error, and the login
  overlay stays up until the document reloads.

### 📱 Android

- New **Settings → IPv6** card with the live switch, public address, warning,
  save action and a confirmation before disabling.
- Relay-agent editor gains **Public IPv6** and **Use IPv6 for this agent**.
- Relay Agents cards are compact enough for two agents without scrolling; the
  list scrolls from the third agent.
- The Traffic History card fits one screen; the explanatory paragraphs above
  Save were removed and the fields were tightened.
- Added API, ViewModel and per-agent switch tests.

### ⚙️ Upgrade notes

- No manual database migration. The new settings and agent fields default off,
  so existing proxied names keep their IPv4-only behaviour until enabled.
- Enabling Proxy IPv6 requires a global IPv6 address. Agents receive the policy
  on their next heartbeat; no agent restart is required.
- Docker images: `ghcr.io/digitrob/digitdns:5.2.2` and
  `robmeister/digitdns:5.2.2`, plus the matching agent images.

## 🏷️ [v5.2.1] — Proxy leak protection

### 🛡️ Proxy leak protection

- Proxied names no longer receive upstream `HTTPS`/`SVCB` answers. The hub
  answers `NOERROR` with no answers, no `AD` and no cache entry, so clients
  cannot discover HTTP/3 or the origin's real IPv4/IPv6 hints and bypass the
  SNI relay.
- Direct names keep normal `HTTPS`/`SVCB`; blocked names already return the
  block response for every query type.
- New live `proxy_leak_protection` setting, default `true`; changing it applies
  without a restart, and turning it off requires an explicit confirmation.
- Added handler, cache, direct, blocked and live-disable tests.

### 🖥️ Web panel

- New **Settings → Proxy Leak Protection** card beside **DNSSEC Validation**.
- The DNSSEC card sits in a two-column row on wide panels, stacks on mobile,
  and keeps its built-in root anchors permanently visible in a fixed-height
  block.
- The DNSSEC and proxy switches stay hidden until their saved state has loaded,
  so a refresh no longer animates them from a default position.
- New authenticated `/api/settings/proxy-leak` route and v3
  `/api/v3/settings/proxy-leak` route for Android and API-key clients.
- A successful sign-in now reloads the document instead of reusing the old
  markup, so an updated panel is visible immediately and the standalone login
  redirect bypasses the back/forward cache.
- Live-log `PROXY · QUIC FALLBACK` rows are treated as proxied queries: the
  **PROXY** filter, the RELAY column and aggregate proxied totals now include
  them.

### 📱 Android

- Renamed the **DNSSEC** Settings section to **DNSSEC/Proxy Leak** and added the
  **Proxy Leak Protection** card below the DNSSEC card.
- Proxy protection is live: the switch saves immediately, shows `ACTIVE`/`OFF`,
  and asks for confirmation before disabling.
- Both cards fit the Settings area without scrolling on a standard phone.
- Pull-to-refresh no longer disables the **Save DNSSEC settings** button or
  clears the saved-status line; refresh and first-load states are separate.
- Added API, ViewModel, repository and routing tests for the live proxy guard.

## 🏷️ [v5.2.0] — DNSSEC validation and panel/app controls

**A feature release.** Full DNSSEC validation in the hub, a Settings card and
v3 API for its policy, and a dedicated DNSSEC section in the Android app. The
feature is opt-in and startup-scoped; existing installs keep their current
behaviour until it is enabled and the daemon restarts.

### 🛡️ DNSSEC validation

- Walks the DS/DNSKEY chain from the built-in IANA root KSKs (`20326`, `38696`)
  through every zone cut and verifies answer RRSIGs.
- Validates NXDOMAIN and NODATA with NSEC and NSEC3 proofs, including wildcard
  expansion and NSEC3 opt-out.
- Sets `AD` only on answers validated locally. Bogus answers return `SERVFAIL`
  with EDE 6; `CD=1` bypasses validation and clears `AD`; unsigned delegations
  are served without `AD`.
- DNSSEC-aware cache keys separate EDNS, DO and CD variants. DO=0 entries cannot
  leak RRSIG/NSEC/NSEC3 records, refresh preserves the entry's flags, and cached
  TTLs cannot outlive the nearest RRSIG expiration.
- Root trust anchors are built in; custom DS/DNSKEY anchors are validated at
  save time. `dnssec_validation` and `dnssec_trust_anchors` are startup-only.
- Added negative/positive validation tests, tampering tests, forged-SOA
  downgrade tests, wildcard tests, cache isolation and signature-lifetime tests.

### 🖥️ Web panel

- New **Settings → DNSSEC Validation** card above **Traffic History**.
- Shows the running state separately from the saved policy, with a clear
  restart-required banner.
- Enable/disable, trust-anchor editor, built-in anchor viewer and save-time
  validation.
- New authenticated `/api/settings/dnssec` route and v3
  `/api/v3/settings/dnssec` route for Android and API-key clients.
- Added web handler tests for round-trip saves, invalid/non-root anchors,
  pasted multi-line anchors, and preservation of unrelated DNS settings.

### 📱 Android

- New **DNSSEC** Settings section immediately before **Traffic History**.
- The card waits for the saved policy before drawing, so the switch opens in its
  real position and the status never flips from `OFF` to `RUNNING`.
- Trust-anchor editor, expandable built-in anchors, restart-required notice and
  save progress.
- `RUNNING`/`OFF` now uses the shared bordered `StatusBadge` style.
- Compact layout fits the Settings area without scrolling, even with the
  built-in anchors expanded.
- Added API, ViewModel and UI-state tests.

### 🧪 Verification

- Full Go suite, `go vet`, cache/DNS/DNSSEC race tests and web tests pass.
- Android `assembleDebug`, unit tests and detekt pass.
- Live verification: signed names carry `AD`, unsigned names do not, bogus
  names return `SERVFAIL`, `+cd` bypasses validation, and signed NXDOMAIN
  carries `AD`.

## 🏷️ [v5.1.4] — Cleaner Top Domains and Docker Hub

### 🎨 Changed

- **Reverse DNS is excluded from Top Domains and Top Blocked Domains.**
  Names under `in-addr.arpa` and `ip6.arpa` no longer bury real domains in the
  ranked tables. They still count toward total queries, blocked/cached/failure
  counters, `ByQType` and the live query log, so PTR volume remains visible on
  the qtype card.
- Existing aggregate buckets are filtered on read, so historical reverse-DNS
  rows disappear immediately instead of waiting for the 90-day retention
  window.
- **Docker Hub mirror.** Both the hub and relay-agent images are now published
  to `robmeister/digitdns` and `robmeister/digitdns-agent` alongside GHCR,
  with the same tags, multi-arch manifests and cosign signatures.

### 🧪 Tests

- Added coverage for IPv4/IPv6 reverse names, blocked reverse names,
  suffix-boundary false positives, and legacy buckets containing reverse rows.
- Full server suite, `go vet` and the aggregate race test pass.

---

## 🏷️ [v5.1.3] — Faster route boot and Android UI

### ⚡ Web

- **Pre-paint tab selection.** A reload of `/clients`, `/logs` or `/settings`
  no longer paints Dashboard first. An inline head script sets the boot tab
  from the URL and CSS renders only that tab until `app.js` applies it.
- **Boot no longer waits on the client list.** `bootDashboard` loads only the
  active tab; the client list loads when Clients is opened.
- **Parallel boot probes.** `/api/auth/me` and `/api/config` now run together
  instead of one after the other.
- **Correct tab survives a config failure** instead of falling back to
  Dashboard.
- The static Dashboard nav highlight is neutralised during boot, so a reload
  does not briefly mark the wrong tab as active.

### 📱 Android

- **Startup work moved off the main thread.** DataStore and Keystore reads run
  on `Dispatchers.IO`.
- **Navigation no longer blocks on a session refresh.** The shell appears from
  cached credentials; `ensureSession()` runs in the background and the 401
  retry path covers a lapsed session.
- **Live Query Log batching.** `LiveQueryStream` publishes UI snapshots at most
  four times a second instead of rebuilding a 200-item list for every SSE
  query. Notification listeners still see every event immediately.
- **Lifecycle-aware polling and streams.** Dashboard stats/history polls and
  the shared live-stream reference stop when their tab is not visible.
- **Lighter list recomposition.** Domain rows, filtered clients and visible
  live-log rows are computed once per relevant state change instead of on
  every read.
- **Live-log scrolling** no longer starts an animation for every new query.
- **Clients appear after the list call**, with config/link data filling in
  concurrently. The duplicate first refresh is gone.
- History top tables request the five rows the phone renders.

### 🧪 Tests

- Added a web test that keeps the pre-paint route map and `routeTabs` in sync.
- Updated CSP and config-loading regression tests for the second inline script
  and parallel boot probes.
- Server suite, web tests and Android unit tests pass.

---

## 🏷️ [v5.1.2] — Backend performance for the resolver and live log

### ⚡ Performance

- **Telemetry fast path.** `StatsService` no longer JSON-encodes and fans out
  every query when no live-log subscriber is connected. The 100-entry history
  is now a fixed ring instead of a slice that periodically reallocated and
  copied the whole window.
- **Batched live-log writes.** SSE frames are collected for 20 ms or 64 frames
  and written with one flush, instead of one write and one flush per query.
- **Typed DNS answers.** Proxy A/AAAA, blocked null-IP, custom-record and
  self-name answers no longer go through `fmt.Sprintf` plus `dns.NewRR` on every
  query. Public addresses and relay agent IPv4s are parsed when configuration
  changes and reused.
- **Compiled policy indexes.** Per-domain resolution profiles are compiled when
  the upstream policy is installed, and relay assignments are indexed by host
  and wildcard when they are saved. Per-query lookup is O(labels) instead of
  O(profiles) or O(assignments).
- **Narrower cache invalidation.** Saving a Direct/Proxy/Block rule no longer
  flushes the whole DNS cache. The cache is flushed only when a resolution
  profile actually changes, because the matcher runs before the cache.

### 📊 Measured locally

- Proxy handler answer: ~3.9 µs / 25 allocs → ~0.6 µs / 5 allocs.
- Blocked handler answer: ~3.6 µs / 24 allocs → ~0.5 µs / 5 allocs.
- Cache-hit handler path: ~2.1 µs / 11 allocs → ~0.65 µs / 7 allocs.
- Profile lookup with 200 profiles: ~41.8 µs → ~0.1 µs.
- A-record construction: ~1.77 µs / 15 allocs → ~39 ns / 1 alloc.

Loopback UDP throughput is limited by the local network stack, so end-to-end
QPS moved only a few percent; the handler CPU and allocation reductions are the
durable gain.

### 🧪 Tests

- Added benchmarks for handler paths, typed A records and compiled profile
  lookup.
- Added tests for ring-buffer order, compiled profile equivalence, relay
  wildcard precedence, and cache survival across a rule-only save.
- Full server suite, race detector for service/web, and vet pass.

---

## 🏷️ [v5.1.1] — Sky-blue Proxy status and grey favicon

### 🎨 Changed

- **Graphite keeps one route colour.** Proxy status is now sky blue
  (`#7DD3FC`) on the web Live Log, the Proxied Domains list and the Android
  Live Query Log and Domains screens. Direct stays green, Blocked red and
  Paused grey/amber; the rest of the panel remains monochrome.
- Proxy domains use a semantic `.proxy-status-text` class on the web instead of
  the shared `text-cyan-300` utility, so the accent does not spread to other
  cyan text.
- Android adds a dedicated `ExtendedColors.proxy` token and a
  `QueryActionTone.Proxy`; informational `info` accents stay neutral in
  Graphite.
- The whitelist-mode chip keeps its neutral Graphite treatment even though it
  shares the `.badge-proxy` shape.

### 🛠 Fixed

- The web favicon is now the grey shield in the dashboard and standalone login
  documents, matching the Graphite launcher icon.

### 🧪 Tests

- Added a colour-token test that pins the Graphite Proxy blue and the neutral
  `info` token.
- Existing web and Android suites continue to pass.

---

## 🏷️ [v5.1.0] — Graphite theme for web and Android

### ✨ Added

- **Graphite**, a neutral third theme for the web panel and Android app. The
  theme button cycles Dark → Grey → Light and shows the current state with a
  moon, half-filled circle or sun.
- **Theme persistence across login and restarts.** The Android login screen has
  the same three-state control as the web panel; the selected theme is stored in
  `AppPreferences` and survives sign-in, process death and relaunch.
- **Grey launcher icon** and grey in-app shield mark.
- **README Graphite screenshots** for the web dashboard and Android login,
  dashboard and theme settings. Public addresses are sanitised.

### 🎨 Changed

- Web selected states are neutral: range chips, filter chips, API bind options,
  snippet tabs, service rows, live-log scopes and the agent architecture
  selector no longer use cyan fills.
- Web primary buttons use a light-grey fill with dark text; soft, ghost and
  icon buttons are neutral. Card top edges, hover borders and shadows are grey.
- Web cyan remains only as a small accent on icons, links, the chart line and
  focus rings.
- Web login and standalone login follow Grey and Light consistently, including
  form borders, placeholders and focus rings.
- Android Material 3 scheme adds Graphite surfaces, neutral primary/secondary
  roles and grey controls while keeping semantic green/amber/red.
- Android DoH uses a soft lavender accent instead of purple or cyan.
- Android theme card grows to Follow system / Dark / Grey / Light and wraps
  cleanly with `FlowRow`.

### 🛠 Fixed

- Web field boundaries raised for 3:1 contrast; login placeholders and focus
  rings corrected.
- Links that lost their colour cue are underlined in Graphite.
- The web pre-paint script and theme state machine accept the third theme.
- The Android login header keeps its layout while gaining a 48dp theme button.

### 🧪 Tests

- Existing web theme, icon and CSP regression tests continue to pass.
- Android `ThemeMode` parsing, system resolution and cycle unit tests.
- Existing Android and server suites unchanged.

---

## 🏷️ [v5.0.4] — Real arm64 hub and agent images

### 🛠 Fixed

- **arm64 images contained amd64 binaries.** `ARG TARGETARCH=amd64` overrode
  BuildKit's per-platform value, so `linux/arm64` layers failed with
  `exec format error`. Both `Dockerfile` and `Dockerfile.agent` now run the
  builder on `$BUILDPLATFORM` and consume `TARGETOS`/`TARGETARCH` without
  overriding them.
- **CI executes both arm64 images under QEMU** after publishing. A manifest can
  claim arm64 while the binary is x86-64; the new smoke test fails the build
  instead of shipping it.

### 🔐 Upgrade note

5.0.3 and earlier ARM images are unusable. Remove the cached image/container and
pull the fixed 5.0.4 tag or `latest` with `--pull always`.

### 🧪 Tests

- Docker workflow YAML validation and real `linux/arm64` `-version` execution
  for both hub and agent images.
- Go/web/Android gates unchanged.

---

## 🏷️ [v5.0.3] — Multi-arch agent setup, made copyable

### ✨ Added

- **Agent architecture selector.** The agent creation box offers Auto, amd64 and
  arm64. The generated `docker run` adds `--platform linux/amd64` or
  `--platform linux/arm64` when a specific architecture is selected.
- **`--pull always`** in the generated agent command, so a cached amd64
  `latest` cannot be reused on an ARM host.
- **Copy button for the Docker run command**, matching the existing copy-key
  control.
- **Docker CI manifest gate** asserting that the published agent index contains
  both `linux/amd64` and `linux/arm64`.

### 🔄 Changed

- Agent-key copying now uses the shared clipboard helper and its fallback.
- Android companion version bumped to 5.0.3 (no functional Android changes).

### 🧪 Tests

- Web regression test for the platform selector, copy button and
  `--pull always`/`--platform` command contract.
- Web Node tests, Go web tests and Docker workflow validation.

---

## 🏷️ [v5.0.2] — Allowed-only Top domains and polish fixes

### 🛠 Fixed

- **Blocked names no longer count in Top domains.** The `domain` Top query
  subtracts each bucket's `TopBlockedDomains` count, so old and new ranges show
  allowed-only counts immediately. Domains that were only blocked disappear from
  the card; `Other` is clamped against underflow.
- **Rewrite queries appear under the Direct filter** on web and Android.
- **Locale-safe casing** for card headings, protocols, summary labels, action
  filters and search normalization.
- **320dp Queries by Relay title** wraps cleanly instead of truncating.

### 🔄 Changed

- Android DNS Rewrite bulk import reads and parses files off the UI thread and
  uses localized status/summary strings.
- Compact domain switch clears the inner switch semantics so TalkBack sees one
  control.
- README screenshots refreshed for the current dashboard and card names.
- Release workflow sets a descriptive release title automatically.

### 🔐 Security and cleanup

- The public test fixture no longer contains a realistic host/subscription token.
- Added a tail-clamp regression test for blocked-domain subtraction.

### 🧪 Tests

- Aggregate mixed allowed/blocked, blocked-only and tail-clamp cases.
- Android unit/detekt/lint/build, web Node tests and the full Go suite.

---

## 🏷️ [v5.0.1] — Dashboard polish, DNS Rewrites and relay clarity

A patch over 5.0.0 with no storage or configuration migration. It unifies the
dashboard typography, completes the DNS Rewrites workflow on Android, and makes
the live log and relay statistics easier to read.

### ✨ Added

- **Android bulk import for DNS Rewrites.** Paste a `hostname address` table or
  choose a file. Supports comments, space/comma/semicolon/`=` separators, strict
  IPv4 and IPv6 validation, update/unchanged detection and invalid-line counts.
  Matching hosts are updated instead of duplicated.
- **Rewrite live-log badge.** A query answered by a DNS Rewrite now carries a
  **Rewrite** badge in the action slot on web and Android.
- **Missing card pictograms.** Summary and Queries over time on Android, and the
  six history KPI cards on web, now carry the same icon language as the rest of
  the dashboard.
- **Range-aware Android chart labels.** Four evenly spaced ticks now describe the
  selected window: times for 1h/24h, weekday/date for 7d, and dates for 30d/90d.

### 🔄 Changed

- **Pinned Records / Custom Static Records → DNS Rewrites** on web and Android.
  Android's **Pin** action is now **Save**. The `custom_records` API and storage
  names are unchanged.
- **Relay Countries → Queries by Relay** on both surfaces. Web shows flag, relay
  name, query count and share bar; Android shows flag, relay name and count.
  **Local** uses the installed server's flag. Relay flags are sized to the row.
- **Unified Android card titles.** All cards use uppercase muted labels at the
  larger card type size with a 16dp pictogram. The compact KPI tiles keep their
  height by using a tighter line box.
- **Protocol display names** are now DoH, DoT, DoQ, DoH3 and DNSCrypt on web and
  Android; UDP/TCP remain uppercase. Protocol colours are shared between the
  live log and the Protocols card.
- **Live Log query count** moved to the top-left of the Updated/Online row in the
  same 8sp shared style, with a fixed slot so it cannot shift.
- **1h history charts** now span the full selected hour and leave future space
  empty; range labels and tick spacing adapt to the selected window.
- **Thinner domain switches.** Visual track/thumb shrink while the 48dp touch
  target and row height stay intact.

### 🛠 Fixed

- Domains **Reload** returned to the left after the shared header slot changed.
- Clients **“+”** returned to the top-left.
- Live-log query count no longer overlaps or drifts from the Updated/Online row.
- History **Summary** and **Queries over time** titles now match the other cards.
- Relay flags no longer crowd the row; the local relay now identifies the
  server's own country.

### 🧪 Tests

- Android unit tests for DNS Rewrite parsing (separators, comments, IPv4/IPv6,
  duplicate/invalid lines, update/unchanged counts).
- Android action-style test for the **Rewrite** label.
- Full Android unit/detekt/lint/build, web Node tests and Go suite.

---

## 🏷️ [v5.0.0] — DNS server, SNI relay and ad blocking in one stack

DigitDNS becomes a complete DNS solution: the resolver and TLS relay are joined
by a DNS sink with a real blocklist engine and a long-window aggregate history.
One install covers DNS resolution, geo-blocking bypass and ad blocking.

### ✨ Added

- **Ad-blocking engine.** A DNS sink that answers blocked names before an
  upstream query, with `0.0.0.0`, `NXDOMAIN` and `REFUSED` response modes,
  configurable block TTL, update interval and an always-allow list. The
  allowlist wins over every blocklist.
- **45 curated blocklist presets** in five groups: General protection,
  Supplementary ads & trackers, Malware/phishing/scams, Device & vendor
  telemetry, and Optional extras. Sources include HaGeZi Pro/Light/Pro+/Ultimate,
  OISD Big/Small, AdGuard DNS, 1Hosts Xtra/Lite, StevenBlack Unified, Peter Lowe,
  AdAway, Dan Pollock, URLhaus and Phishing Army. Every row carries its
  description, licence, warning, homepage and raw file link; a recommended base
  can be installed in one action.
- **Custom blocklists.** Subscribe to a hosts/Adblock URL or upload a local file.
  Downloads are capped at 64 MB and uploads at 16 MB, parsed off the DNS hot path
  and installed atomically. Duplicate URLs are rejected and names are bounded.
- **Per-list lifecycle.** Enable/disable without deleting, update one list or all
  lists, delete, view unique domains, rule count, allow/skipped entries and last
  update, and scan preset sizes in the background before installing.
- **Active-first lists.** Enabled lists are pinned under an **ACTIVE** heading at
  the top of the web and Android list views and removed from their catalogue
  groups, so the lists actually in force are immediately visible.
- **Web Ad Blocking tab.** Policy card, blocklist roster, search, custom
  URL/file panel, active counts, refresh and per-list actions.
- **Android Ad Blocking screen.** The same policy, preset catalogue, custom URL
  and file upload, active-first ordering, green active states and update/delete
  actions, under **Settings → Ad Blocking**.
- **Five-minute 1h statistics.** The current hour is split into twelve real
  5-minute slices (queries, blocked, proxied, cached, failed and latency). The 1h
  series returns the started slices, so web and Android draw a line instead of a
  single point; 24h/7d read hourly buckets and 30d/90d read daily buckets.
- **v3 filtering API.** Status, settings, presets, list creation, enable/disable,
  update and delete under `/api/v3/filtering`.
- **Free dashboard layout.** Cards move anywhere on a 12-column, 12-pixel-row
  grid, resize from every edge and corner, and can cross the metrics, main and
  history groups. Layout version 3 migrates the previous document; a reset action
  sits beside the edit control.
- **Sticky history toolbar.** Refresh statistics on the left, status, range chips
  and CSV/JSON export actions on the right; the row stays pinned while cards
  scroll underneath and is outside every editable grid.
- **1h range chip** on web and Android, plus a single-bucket bar fallback.
- **Android header controls.** Range chips and the aggregate update clock sit in
  the top row beside Online; the same clock appears in the same place on the
  other live-backed screens.
- **Numeric history chart axis.** Android’s Queries over time card now draws
  peak/mid/zero labels and gridlines.
- **Compact Android summary.** The six aggregate metrics render as two rows of
  three with tighter type and spacing.

### 🔄 Changed

- **Web export buttons** moved after the range chips to the far right and use the
  green action tint, separating them from time selection.
- **Android history range** is a top-header control instead of a dashboard card;
  the Traffic History card and its “no raw queries retained” subtitle are gone.
- **Android updated clock** no longer flips to “Loading…” on a background poll,
  eliminating the flash and the chip-row jump.
- **Responsive KPI cards.** The six live cards wrap their meta columns, CPU line
  and traffic readings instead of clipping at narrow widths; the six-across
  layout is deferred to `2xl`, and free placement starts at 1280 px.
- **History card layout.** Android summary is three-up; the chart has a real
  scale; ranked cards and the layout editor keep their existing behaviour.
- **Docs and OpenAPI** describe the blocklist routes, 5-minute interval and the
  expanded v3 surface.

### 🛡 Security and privacy

- Blocklist content is fetched at runtime from upstream projects and never
  bundled; subscriptions and policy live in the encrypted local database.
- Statistics stay aggregate-only: no raw query log and no replayable DNS payload.
- Existing client anonymization, per-store HMAC salt and opt-out remain unchanged.
- List downloads have size caps and are parsed away from the DNS hot path.

### 🧪 Tests

- Blocklist parser and allowlist precedence, filter service lifecycle, preset
  catalogue and count scanning.
- Filtering API/session routes and web document controls.
- Five-minute aggregate chunks and 1h series shape.
- Android filtering API, roster ordering, view-model state, dashboard header and
  layout tests.
- Full Go suite/vet/race, web Node tests, Tailwind purge and Android
  unit/detekt/lint/build gates.

### ⬆️ Upgrade

No database migration. `stats.db` starts recording 5-minute slices at first
start; the current hour may only show slices recorded after the restart. Pull
`ghcr.io/digitrob/digitdns:5.0.0` and `ghcr.io/digitrob/digitdns-agent:5.0.0`,
then recreate the container.

---

## 🏷️ [v4.1.0] — Traffic History

The first minor release after 4.0 adds an optional 90-day aggregate history,
with dashboard cards on web and Android, privacy controls and a v3 API. DNS
resolution, relay forwarding, certificates and subscriber accounting keep their
existing behaviour.

### ✨ Added

- **Aggregate history store.** A separate `stats.db` keeps hourly and daily
  counters and bounded top-N tables — never the raw query log. Buckets cover
  queries, blocked/proxied/cached/failed totals, action, protocol, upstream,
  qtype, rcode, relay country, latency and six ranked dimensions.
- **Retention and rollups.** Hourly buckets are kept for 7 days and rolled into
  daily buckets; daily history defaults to 90 days and is pruned automatically.
- **Bounded cardinality.** Top-N defaults are 1,000 domains and 500 clients per
  bucket, with overflow counted as `other` rather than growing without limit.
- **HyperLogLog distinct clients.** Every bucket stores 256 registers, merged
  across buckets, so “active clients” is not clipped by the top-N table.
  Pre-HLL ranges report `top-N approx.` instead of pretending precision.
- **Client privacy controls.** Optional HMAC-SHA256 client identifiers, keyed by
  a per-store salt, plus a per-client opt-out for up to 500 IPs or account
  names. An excluded client still contributes to totals but no identifier is
  retained. Existing aggregates are never rewritten.
- **Traffic History dashboard.** Six KPI cards, a stacked Allowed / Blocked /
  Proxied chart, and ranked top domains, blocked domains, clients, upstreams,
  protocols and relay countries. Ranges: 24h, 7d, 30d, 90d.
- **Layout-editor integration.** History cards drag, resize, hide, reorder and
  persist with the live dashboard. On Android they participate in the existing
  card order/visibility editor.
- **CSV and JSON export.** Download the selected range as a CSV time series or a
  full JSON document from the web dashboard.
- **v3 history API.** `GET /api/v3/stats/{summary,series,top,export}` plus
  `GET/POST /api/v3/stats/settings`, all documented in OpenAPI and reachable
  with either a dashboard session or the master API key.
- **Android 4.1.0 companion.** Range selector, summary, chart, ranked tables,
  relay flags, client names and a **Settings → Traffic History** screen for
  retention, top-N, anonymization and client opt-out.
- **Client names in ranked rows.** The top-client dimension resolves a stored IP
  to the matching client record’s current name at response time; unknown or
  deleted clients keep the IP. JSON export gets the same treatment.
- **Relay flags.** Relay-country rows show the bundled flag and localized
  country name on web and Android.

### 🔄 Changed

- **Ranked history cards use their space.** Web cards pin the header and
  `Other N` footer, scroll only the entry list, and grow the list viewport when
  the card is resized taller. Default card height shows five entries.
- **Web row treatment matches upstream resolvers.** History rows use the same
  inset `slate-950/70` tile and `slate-800` border, so lists read as one panel.
- **Android card geometry.** Ranked tables are full-width except Protocols and
  Relay countries, which share one half-width row. Top domains, blocked domains
  and upstreams show five rows; clients, protocols and relay countries show
  three. Only the entry list scrolls; every card has a fixed height and tighter
  bottom padding.
- **Settings and API docs** now describe the aggregate policy, retention,
  privacy limits and the new v3 routes.

### 🛡 Security and privacy

- `stats.db` is created beside `data.db` with mode `0600` and contains only
  aggregates.
- No raw query, client query or replayable log is written to disk.
- Client anonymization uses a per-store salt kept inside `stats.db`; it protects
  casual disclosure, not an attacker who already has the database.
- Per-client opt-out matches both source address and account name, so either
  identifier suppresses the client key.

### 🧪 Tests

- Aggregate store, rollups, pruning, HLL merge/estimate, anonymization, opt-out,
  flush-race, query API, v3 handlers, export and OpenAPI coverage.
- Web history markup, refresh/range/export, layout integration, escaping and
  fixed-card list behaviour.
- Android DTO/API mapping, client-name fallback, history state and layout
  defaults, plus full unit, detekt and lint runs.

### ⬆️ Upgrading

- **Docker:** pull `ghcr.io/digitrob/digitdns:4.1.0` and recreate the container.
  Keep `/app/data` and `/app/certs`. `stats.db` is created automatically on
  first start and begins collecting from that moment.
- **Binary/installer:** replace the binary or rerun the installer, then restart.
  Existing `data.db`, `master.key` and configuration are untouched.
- **Android:** install `android-v4.1.0` APKs from the v4.1.0 release page.
- **History settings:** retention, top-N, anonymization and opt-out are
  restart-scoped; saving tells you a restart is required.

---

## 🏷️ [v4.0.3] — A sharper panel

A dashboard-only patch. No DNS resolution, relay, API, database or configuration
behaviour changed.

### ✨ Added

- **Live Log refresh control.** A refresh button sits to the left of the search
  field. It reconnects the SSE stream, reloads the server's recent-query
  snapshot, keeps the active search/filter/sort, and resumes a paused log.
  The button spins while connecting, disables itself against stacked reconnects,
  and falls back to a warning if the snapshot does not arrive.
- **Shared pause-state helper.** The pause button, its `aria-pressed` state and
  the refresh path now use one owner instead of duplicating the DOM update.

### 🔄 Changed

- **Dashboard brand mark.** The sidebar and mobile header use the mobile app
  icon, cropped to remove the outer dark-blue padding and enlarged to 48px on
  desktop and 36px on mobile.
- **Compact Settings layout.** The Settings tab is a responsive card grid:
  two columns at ≥1100px, one below. Wide forms keep a full row, while the
  simple cards share a row. Padding and section gaps are reduced without hiding
  fields or status text.

### 🧪 Tests

- Refresh-control placement and Settings-grid markup are pinned by new web
  tests.

## 🏷️ [v4.0.2] — A truthful allowlist count

A one-line follow-up to 4.0.1. The agent’s client allowlist can be the literal
entry `any`; the status line counted only explicit CIDRs, so an allow-all agent
printed `0 client entries` and warned that every connection would be refused —
while it was actually relaying normally. `Len()` now counts `any` as one rule.

No relay, catalogue or configuration behaviour changed. The warning is simply
gone when the hub is in allow-all mode.

## 🏷️ [v4.0.1] — Quiet agents under internet noise

A relay agent on a public IPv4 is found by scanners within minutes. Because the
hub in this release was running in allow-all mode, the agent accepted those
connections far enough to read their destination, then refused every one that
named a bare IP — and wrote a log line for each attempt. A single probe loop
could bury the useful relay lines under thousands of identical refusals.

This patch keeps the same access model and the same routing behaviour, but makes
the log usable.

### 🔄 Changed

- **Refusal logs are throttled.** The first refusal for a source and reason is
  printed immediately; repeats are folded into at most one line per minute with
  a `N similar events suppressed` count. The map is bounded, so a scanner cannot
  grow agent memory by inventing new peers.
- **Bare-IP requests are counted separately.** A request whose destination is an
  IP literal has no hostname to relay to, so it is now counted as `bare-IP`
  instead of being mixed into host-allowlist refusals. The 60-second summary
  reports it alongside active, total, refused and unreadable counts.
- **No valid relay traffic changed.** Domain requests that pass the allowlist,
  client checks, heartbeats, catalogue sync and traffic accounting are untouched.

### 🐛 Fixed

- **The agent log no longer grows one line per probe.** A public agent can still
  be scanned, but the log now reports the pattern and the suppressed count
  rather than the whole stream.

### ⬆️ Upgrading

No database or configuration change. Replace the hub binary or pull
`ghcr.io/digitrob/digitdns:4.0.1`; update the agent with
`ghcr.io/digitrob/digitdns-agent:4.0.1` (or the new amd64/arm64 binary). The
Android app remains 4.0.0 and is compatible.

## 🏷️ [v4.0.0] — Geo relay agents: another country, without another hub

DigitDNS can now send chosen domains out through a small relay agent on a VPS in
another country. The hub keeps the panel, the subscriber database, the
certificates and every credential; the agent only carries the encrypted stream.
That is the headline, but the release also gives the feature a real home on both
clients and fixes several rough edges found while building it.

This is a major version because the product gained a second deployable role and
a new control surface — not because existing installs must change anything.
A hub with no agents behaves exactly as it did in 3.8.0.

### ✨ Added

- **A relay agent you can install anywhere.** `digitdns-agent` is a separate,
  deliberately small binary with no panel, database or DNS server. It enrols
  with a key, pulls a host and client allowlist from the hub, relays TCP/TLS
  byte-for-byte, and reports traffic every 30 seconds. The hub learns the
  agent's public IPv4 from the connection, not from a claim in the payload.
- **Settings → Relay Agents on the web.** Add, edit, enable/disable, delete,
  rotate a key, copy the key and run a real connection test. Each card shows the
  country flag, online/offline/disabled state, public IP, masked key, version,
  last heartbeat, assigned domains, relayed traffic and the services using it.
  The edited card is outlined in cyan with an `EDITING` badge so it is obvious
  which card the form belongs to.
- **Relay control on every domain row.** The web domain table has a relay
  chooser; a domain can inherit its service default, force **This Server**, or
  name an agent. The Android row puts the same choice behind an icon-only relay
  flag on the second line, so the gear, status pill and switch never move.
- **Service defaults and bulk Egress.** A service can carry a default relay, and
  a selection of domains can be pointed at one exit in a single action. The
  precedence is: domain override, then service default, then This Server.
- **A genuine connection test.** **Test connection** opens a TCP connection to
  the agent's advertised relay port, measures the round trip, stores
  `ping_ms`/`ping_at`, and shows green under 40 ms, amber over 40 ms and red over
  80 ms — the same palette as the upstream resolver rows. A failed test says what
  address it tried and why it failed.
- **Agent keys that can be managed safely.** Create returns the clear key once;
  the server also stores an encrypted copy for later authenticated copying.
  Rotation invalidates the old key immediately. Agent list responses never
  include a clear key. A key created before encrypted storage existed answers
  409 and needs one rotation.
- **Agent lifecycle controls.** Disabling or deleting an agent stops its relay
  immediately and prunes its assignments, so DNS answers fall back to This
  Server instead of naming an agent that no longer exists.
- **Android parity.** **Settings → Relay Agents** sits beside Servers. The cards
  are client-card shaped: flag, name, status, edit/pause/delete, copyable public
  IP, masked key with rotate and copy, version, last seen, domains, traffic and
  service defaults. The page is one card wide on a phone and two across when the
  window is wide enough.
- **Agent binaries and a container image.** Release assets now include
  `digitdns-agent-linux-amd64` and `digitdns-agent-linux-arm64`, and the
  multi-arch `ghcr.io/digitrob/digitdns-agent` image is built and cosign-signed
  by the same workflow as the hub image.
- **v3 agent API.** `GET/POST /api/v3/agents`, `PUT/DELETE /api/v3/agents/{id}`,
  `POST …/{id}/enable|disable|rotate-key|test` and `GET …/{id}/key` expose the
  same operations the buttons use.

### 🔄 Changed

- **Relay cards use the panel's card language.** Public IP and Agent Key occupy
  a narrower left column; Version, Last Seen and Domains stack in a compact
  right rail; the traffic line sits under the key instead of adding another
  full-width band. The web grid fits two cards per row on a normal desktop and
  three on a wide one.
- **Android domain rows are icon-first.** The service favicon and relay flag are
  the controls; tapping them opens the service or relay picker. The wildcard
  label stays as text, and the row height and right-side controls are unchanged.
- **The Online badge left the Android Relay Agents page.** The freed space now
  shows `N agents · M online`, which is the information that page is about.
- **The Android Relay Agents page is a single scroll container.** Its header and
  Add agent row are part of the refreshable grid, so pull-to-refresh works from
  the top of the page, and the scaffold no longer adds the status/nav bar insets
  twice.
- **Dark-mode dialogs follow the app, not the phone.** Shared dialog and menu
  surfaces now resolve from the app's card tokens, so pinning the app to Dark on
  a Light phone no longer produces a white menu with pale text.
- **The agent test is a real network check.** It is not a heartbeat-age guess;
  an offline-looking agent with a reachable TCP port can still be diagnosed.

### 🐛 Fixed

- **Relay assignments survive a rules save.** The Android repository was building
  its rules request without `domain_agents` or `service_agents`, so a chip change
  reported success and saved nothing. Both maps are now sent with the other rule
  maps.
- **The v3 rules save preserves relay maps.** Partial saves start from the
  persisted state, and the relay maps are written in the same transaction as the
  domain lists and service labels.
- **Legacy agent keys explain themselves.** A 409 from the copy action now tells
  the operator to rotate instead of surfacing the raw server wording.
- **The Android relay picker shows the selected row.** An empty stored value is
  drawn as **Inherit service**, so the check is on the option that is actually in
  force.
- **Service-key deletion removes its relay default.** Deleting a service no
  longer leaves a `service_agents` entry pointing at a service id that no longer
  exists.
- **The web relay grid tracks the window.** Two columns at 768 px, three at
  1280 px, one below; cards no longer stay full-width in a two-column layout
  after a resize.

### ⬆️ Upgrading

The database schema is migrated automatically: agent records, encrypted keys,
ping measurements and the relay assignment maps are created or extended on first
start. Existing proxy/block/direct behaviour is unchanged; an install with no
agents keeps using This Server.

- **Hub binary:** replace `digitdns` and restart. The existing database, master
  key and config are compatible.
- **Hub Docker:** pull `ghcr.io/digitrob/digitdns:latest` (or `4.0.0`).
- **Agent:** install the new `digitdns-agent` binary or pull
  `ghcr.io/digitrob/digitdns-agent:latest`. Keep `/data/agent-state.json`.
- **Android:** version 4.0.0 (versionCode 400), attached to the same release.

Back up `data.db` and `master.key` before upgrading, as always.

## 🏷️ [v3.8.0] — A dashboard you can rearrange, and services that stay out of the way

This release is mostly about finding things. The dashboard can be rearranged to
match the way you actually read it, and the Domains page gained a lightweight
way to group names without adding another routing concept.

### ✨ Added
- **Dashboard layout editing.** A pencil sits beside **Dashboard** in the sidebar. Click it to enter edit mode, drag cards into a different order within their section, and pull the bottom-right grip to change width and height. Click the struck-through pencil to leave; the arrangement is saved on the server.
- **The layout follows you.** The arrangement lives in the install's custom policy store as `dashboard_layout`, is returned by `/api/config`, and is written through `POST /api/dashboard/layout`. It is not tied to one browser's local storage.
- **Mobile reordering.** On a phone, the sidebar is hidden, so the same toggle appears as a small pencil badge on the **Dashboard** tab. Long-press a card to pick it up and move it; sizes keep the normal stacked layout.
- **Bulk service assignment.** Select any number of domains and use **Assign service** to label them in one go, or choose **No service** to clear the label. The menu uses the same favicons and letter marks as the Services panel.
- **Icon search while creating a service.** Type a domain to use its favicon, paste an image URL, or search a word such as `tv`, `film` or `netflix`. The server proxies Iconify searches so the dashboard does not need to open its CSP to the icon host; colourful sets are ranked above monochrome ones, and monochrome icons are tinted to match the panel.
- **Service filters on the domain table.** The Services sidebar is a real filter: click a service to see only its domains, click **No service** for the unlabelled ones, or click the active service again to clear the filter. The count updates with the search and status chips.

### 🔄 Changed
- **The Domains toolbar is one row.** Sort, search, selected count, **Assign service**, Pause, Resume and Remove now share the same row on a wide screen. The search field is fixed-width so the buttons do not get pushed around, and the row wraps cleanly on smaller screens.
- **The Services panel stays put.** It scales with the viewport instead of sitting at a fixed 190 px, and it remains visible while a long domain list scrolls. The page uses the full width of a wide monitor.
- **The domain list has its own scroll area.** The sticky header stays opaque, so rows no longer show through it, and the page itself does not keep growing as domains are added.
- **Service icons fall back honestly.** A missing or broken icon becomes the same coloured letter mark the sidebar uses, instead of an empty image box.
- **Android selection and services.** Long-press starts a selection, the selection bar carries Pause, Resume, Remove and **Assign service**, and the service filter and picker use the same labels and icons as the web panel. Chips stay on one row where the screen allows it.
- **Legacy browser keys and strings are gone.** The panel no longer reads or writes `hyperdns_token` or `hyperdns_theme`, and the last inherited `HyperI18N` strings and old product-version comments were removed.

### 🐛 Fixed
- **A partial rules save no longer deletes service labels.** Toggling a domain (or saving one list) used to write empty `services`, `domain_services` and `service_icons` maps back to the database, so every label silently disappeared. Those maps are now loaded from the persisted rules before the partial save is applied, with a regression test covering the case.
- **Icon search failures do not break the form.** An unavailable icon service leaves the text field usable and the service can still be saved without an icon.
- **The dashboard chart resizes with its card.** Changing the chart card's span or height triggers the chart's own resize pass instead of leaving the canvas at its old size.

### ⬆️ Upgrading

No database migration and no configuration change. The dashboard layout is created the first time you press the pencil, leave edit mode and the save succeeds. Docker users can pull `ghcr.io/digitrob/digitdns:latest` or `3.8.0`; binary installs replace `digitdns` and restart the service. Android is version 3.8.0 (versionCode 380) and updates independently. Back up `data.db` and `master.key` before any upgrade.

## 🏷️ [v3.7.1] — Use the whole window and keep Services in view

### 🔄 Changed
- **The Services column grows with the screen.** It now scales from 220 px to 340 px instead of staying at 190 px, and the page gutters are halved with the maximum width removed, so the Domains panels actually use a wide monitor.
- **Services stays pinned while the domain list scrolls.** The app shell's `overflow-x-hidden` made the shell a vertical scroll container, so `position: sticky` resolved against it and the Services box scrolled away. `overflow-x: clip` hides horizontal overflow without creating a scroller.

## 🏷️ [v3.7.0] — Services: names for domains, not another routing rule

### ✨ Added
- **Services on the web and Android.** A service is a label a domain can carry, stored as `services`, `domain_services` and `service_icons` beside the other rules. It is purely organisational: proxy, block and direct decisions are untouched.
- **A Services panel and filter.** Create, rename and delete services; assign one from a row; filter the Domains table by service or by the absence of one.
- **Icons without a separate icon account.** A bare domain uses its DuckDuckGo favicon, an image URL is used as-is, and an absent or broken image becomes a coloured letter mark.
- **Android service picker.** The same data appears in the toolbar filter and on each domain row, with **Manage services…** available even when the list is empty.

### 🐛 Fixed
- **Service favicons render in the panel.** The dashboard CSP now permits the remote images the favicon URLs point at; without it every image was refused and every service fell back to a letter.

## 🏷️ [v3.6.1] — One login, honest numbers and a readable README

A patch release for the things you notice every day: signing in once, seeing
real per-client query counts, and finding out how to install or fix things.

### 🐛 Fixed
- **The web panel asked for the password twice.** The standalone login page saved the new session under the old `hyperdns_token` key while the dashboard read `digitdns_token` first, so a stale token in the canonical key made the first visit after login look unauthenticated. The page now writes the canonical key and clears the legacy one.
- **Light/Dark could revert on reload.** The theme module kept writing the pre-rename storage key. One key, one migration, and the choice survives a reload.
- **Android “Queries” was always 0.** `total_queries` existed on the record and in the API but nothing ever incremented it. The resolver now counts each lookup for a recognised account in memory and the usage flush persists it, so the client card, the web panel and v3 API show a real figure.
- **The Android live-log popup offered Remove.** Proxying, blocking and direct routing stay there; deleting a domain remains a Domains-row or Remove Selected action, matching the web panel.

### 🔄 Changed
- **One navigation vocabulary on the web.** Dashboard, Domains, Clients, Live Log, API, Settings and Guide read the same on desktop and mobile; the Domains badge says PROXY like the rest of the status set.
- **Android strings have a home.** Bottom navigation, Settings sections, Clients and the client editor now read from `strings.xml`, the foundation for translation and consistent copy.
- **No more “Gaming PC” copy.** Windows setup says Windows 10 / 11, and placeholders/examples and the provisioning text no longer assume a gamer profile.

### 📚 Documentation
- **README rebuilt** around a five-minute quick start, every install path (installer, Docker, binary + systemd, source, offline bundle, Android), first-run and ports, feature deep-dives, operations, backup/restore, troubleshooting, FAQ and security.
- **Corrected claims:** DoQ/DNSCrypt are upstream transports, not client listeners; release binaries are Linux-only.
- **New visuals:** dashboard, Domains, Clients, subscriber portal, two Android screens, an architecture diagram, a four-frame tour GIF and a social preview. All personal data, IPs, hostnames, IDs, UUIDs and credentials are blurred or replaced with demo values.

### ⬆️ Upgrading

No migration or configuration change. Docker users pull `ghcr.io/digitrob/digitdns:latest` (or `3.6.1`); binary installs replace `digitdns` and restart; Android is version 3.6.1 (versionCode 361). Back up `data.db` and `master.key` before any upgrade.

## 🏷️ [v3.6.0] — A client area that feels like the app

Subscribers can now use a proper native client area on Android instead of being
sent to the portal in a browser, and the web panel's login got a little easier to
live with. No migrations and no configuration changes.

### ✨ Added
- **A native client area on Android.** Sign in with the subscription link your provider gave you and get three tabs — **Status**, **Connect** and **Settings** — with the same cards, headings and typography as the admin app. Status shows the account, a green Device IP, traffic and expiry at a glance; Connect lists the registered addresses, the DNS/DoH/DoT endpoints, and the complete per-platform setup guide; Settings holds the subscription, appearance and account controls.
- **Registrations without the portal.** The Registration Secret and Registered Address controls live on the Status tab: enter the secret, register this device or type another address or subnet, and the result appears in Connect. The secret can be saved on the device.
- **Saved subscriptions in Manage Servers.** A subscriber session started from **Add server → User** is kept with the account name the portal reports, appears above the saved servers, can be reopened with one tap, and can be removed with its secret and certificate pin. The same entry is at the top of the dashboard's server switcher.
- **Switch to Admin.** When an admin account is still signed in, subscriber Settings offers a one-tap return to the dashboard; with no admin account the button is visibly disabled and says why.
- **Remember me on the web login.** Both the standalone login page and the dashboard overlay remember the username and password on request, prefill them next visit, and forget them the moment the tick is cleared. The credentials stay on that browser only.
- **Save key on the Android admin login.** API-key sign-in now has a **Save key** tick beside **Show key**. Leave it off and the key works for this run only; a relaunch asks again. Change the key or the admin password and the remembered copy follows.
- **Copy controls everywhere in the client area.** ID, UUID, device IP and every endpoint value copy with a pictogram; the subscription link in Settings does the same.

### 🔄 Changed
- **Client-area cards use the panel's headers.** Uppercase `labelSmall` titles with the same 14 dp glyph as the admin dashboard, on Status, Connect, Guide and Settings. Card copy is unchanged.
- **The Status page fits without scrolling.** Traffic stays content-sized and the registration card takes the remaining height, so the screen is always filled and nothing is cut off — even with the larger green device IP the portal uses.
- **The top-right pill means connectivity.** It reads **Online** or **Offline** for the server connection, while the Account card keeps the subscription's own status: **Active**, **Suspended**, **Expired** or **Quota spent**. Both pills sit on one vertical line.
- **Registered addresses are a list, not chips.** The Connect card fills the tab, lists every address and subnet comma-separated, and scrolls only that text — the page and its header stay still.
- **The web Domains Remove button works on every status.** Blocked and direct rows now disappear too, along with the hidden wildcard twin and the “Added” stamp. A rejected save re-syncs from the server instead of leaving a phantom row.
- **Equal-width resolution pickers.** Transports, Default upstream and Strategy are one grid, so all three dropdowns are exactly as wide as the Default upstream one.
- **The subscription portal no longer shows a Games and services box.** Policy inheritance is unchanged; the card is simply gone.

### 🔴 Removed
- **Remove from the shared quick-action menu.** Proxying, blocking and direct routing stay there; deleting a domain belongs to the Domains row button and Remove Selected, where the managed list is.
- **The portal's “Games and services enabled on this subscription” card**, its “Full Inherit” chip, and the associated copy.

### 🐛 Fixed
- **Subscriber login from Add server did nothing.** The route was missing the callback, so the link verified and the app stayed on the form. It now opens the client area.
- **Removing a blocked or direct domain appeared to do nothing.** The row handler only edited the two proxied lists; it now clears all four and their timestamps.
- **A subscription link with a line break or stray space failed to parse.** Android strips whitespace before parsing and explains when only the token was pasted.
- **API keys were always saved, tick or no tick.** The new Save key choice is honoured, and a failed attempt no longer leaves a rejected key behind.

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
