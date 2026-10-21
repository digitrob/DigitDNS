# DigitDNS — Android companion app

A native Android client, on parity with the web panel, built with Jetpack Compose
and Material 3. It talks to the same server over the **v3 REST API**
(`/<admin-path>/api/v3/…`) documented in `internal/api/openapi/v3.json`.

This directory is a self-contained Gradle project. The Go build, the Docker image
and the dashboard do not read it, and it does not read them — the only contract
between the two is the v3 API and the design tokens copied from
`web/css/style.css` into `ui/theme/PanelColors.kt`.

## Status

**Companion rework complete** — five bottom tabs on full panel parity, multi-server
support, a shared live query stream powering both the Live Query Log and notifications,
a three-way theme, and a signed `android-v*` release workflow. Build, tests, detekt and
lint green.

| Phase | Scope | State |
| --- | --- | --- |
| 0 | Scaffold, theme, Gradle wrapper, CI-ready build | ✅ |
| 1 | Connection + auth (session login + TOTP, master key, TOFU cert) | ✅ |
| 2 | Dashboard, Clients CRUD, Connect (QR) | ✅ |
| 3 | Rules, custom groups, policies, access, upstreams | ✅ |
| 4 | Settings, subscription, 2FA, TLS/ACME, diagnostics, update | ✅ |
| 5 | SSE local notifications, `android-v*` signed APK release | ✅ |
| Rework | 5-tab shell, multi-server, shared live stream, Live Query Log, theme | ✅ |
| Parity | Dashboard card grid + per-upstream stats, Domains list, session auto-refresh, Connect Guide | ✅ |
| UX pass | Swipe-to-delete, scope-aware route menu, bulk import, dropdown filters, Updates folded into About | ✅ |
| UX pass 2 | Status-filtered Domains, pill-triggered menus, drag-reorder cards/upstreams, Diagnostics inline latency | ✅ |
| UX pass 3 | Toggleable Domains chips (+ Direct), swipe Undo, pinned header/badge, compact Subscription/REST API, sort direction glyph | ✅ |

### Navigation

The signed-in shell is five bottom tabs, chosen to fit the panel's seven tabs onto a
phone by promoting the ones a phone acts on and folding the rest into Settings & SSL:

```
Dashboard · Domains · Live Log · Clients · Settings
```

`Domains` is the proxy surface: the panel's routing rules, filtered by status chips
(`Proxy` / `Blocked` / `Paused` / `Direct`). The chips are toggles — tapping the active
chip clears it, and with no chip selected the list shows every status (the `Direct` chip
surfaces the direct-resolved set, which otherwise has no rows of its own). Each row has a
one-line domain with an oval status pill (`Proxy` cyan, `Blocked` red, `Paused` muted,
`Direct` green); tapping the pill opens the quick-action menu that moves the domain to
`Proxy` / `Block` / `Direct` or removes it, under the panel's match scope (`exact` /
`*.host` / `*.parent`) with the same pictograms the web menu uses. A pause/resume switch
flips a proxy rule to `Paused` (no rule applied; the default action runs); the switch slot
is reserved on every status so the pills stay aligned. Search sits beside a sort dropdown;
**Bulk import** pastes or uploads a name list. Deletion everywhere (domain rows, pinned
records, saved servers, diagnostics servers, upstreams) is a right-to-left swipe with a
plain red tray — no trash glyph — and a short-window **Undo** on the Domains and Upstreams
lists. `REST API Gateway` and `Connect Guide` are sections inside **Settings** rather than
peers of the bar; client detail is a pushed destination reached from the list.

Screens that carry the live-connection pill (Dashboard, Domains, Live Log, Clients,
Upstreams, Diagnostics, Subscription, REST API, …) pin a shared `PanelScreenHeader`
above the scrolling content. Pages carry no in-page title — the bottom tab (or the
Settings chip row) already names the screen, so a second copy only wasted vertical
space. The header is a slim strip whose only fixed element is the connection badge
pinned to the far top-right; any screen actions (Clients **+**, Domains **Reload**)
sit to the badge's left, so the badge stays in the same corner on every live-backed
screen and remains visible while the list scrolls. Settings sub-screens carry the
same header shape but without the badge. Client detail is the exception: no title and
no top strip — the system back gesture is the only exit.

Every server-backed screen (Dashboard, Domains, Live Log, Clients, Upstreams,
Settings, Security, REST API, Servers, Access, Connect, Diagnostics, Subscription,
SSL/TLS, Groups, Policies, Alerts) supports pull-to-refresh via the
shared `PanelRefreshBox`. Local-only screens (App settings, About, Login, Client
detail) do not. The former **Updates** page is gone: its System and Update cards live
on **About & Legal**, and the cache-flush card lives on **REST API Gateway**.

## Requirements

- **JDK 17** — `JAVA_HOME` must point at it.
- **Android SDK** — platform `android-36`, build-tools `36.0.0`. Create
  `local.properties` with `sdk.dir=/path/to/Android/sdk` (git-ignored), or rely on
  `ANDROID_HOME`.
- No system Gradle: the committed wrapper (`./gradlew`) downloads the pinned
  distribution.

## Build and test

```sh
cd android
./gradlew :app:compileDebugKotlin      # compile
./gradlew :app:testDebugUnitTest       # JVM unit tests
./gradlew :app:lintDebug               # Android lint
./gradlew :app:assembleDebug           # debug APK
./gradlew detekt                       # Kotlin style report
```

The debug APK lands at `app/build/outputs/apk/debug/app-debug.apk`.

## Versioning and release

The app versions independently of the server. `v3.2.x` tags drive the Go release;
the app uses its own `android-v*` tag namespace and a signed APK published to
GitHub Releases. `versionName` / `versionCode` live in `app/build.gradle.kts`.

`.github/workflows/android.yml` runs the tests, detekt and lint on any change under
`android/`, and on an `android-v*` tag signs and publishes the APK. Its `v*`-style
trigger is distinct from the server's, so the two products release on their own
cadences.

A release ships one APK per CPU architecture plus a universal fallback, each with a
paired `.sha256`:

```
digitdns-companion-<version>-arm64-v8a.apk     # phones since ~2016
digitdns-companion-<version>-armeabi-v7a.apk   # older/low-end ARM
digitdns-companion-<version>-x86_64.apk        # emulators, some tablets
digitdns-companion-<version>-universal.apk     # all ABIs — larger, use if unsure
digitdns-companion-<version>-<abi>.apk.sha256
```

`<version>` is the tag without its `android-v` prefix (`android-v0.1.0` → `0.1.0`).
The per-ABI split (`splits.abi` in `app/build.gradle.kts`) keeps each download to
only the native code the device runs.

Release signing is driven entirely by CI secrets — nothing hardcodes a keystore,
and a local `assembleRelease` without them produces an unsigned APK:

- `ANDROID_KEYSTORE_BASE64` (base64 of the `.jks`; or a local `android/keystore.properties`)
- `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`

The signature is load-bearing: Android refuses to install an update whose signing key
differs from the installed one, so the release job fails rather than publish an APK
signed with the wrong (or no) key.

## Architecture

```
app/src/main/java/com/digitrob/digitdns/
  DigitDnsApp.kt            Application + Hilt entry
  MainActivity.kt           single activity
  core/network/             Retrofit + OkHttp + kotlinx.serialization, v3 API
  core/security/            TOFU certificate trust, Keystore-backed storage
  core/notifications/       SSE frame types + foreground notification service
  data/                     repositories per v3 resource
  data/stream/              the single shared SSE socket (LiveQueryStream)
  domain/model/             DTOs mapped from openapi/v3.json
  ui/theme/                 Colour/Type/Theme ported from web/css/style.css
  ui/<screen>/              one package per panel tab
```

### Multiple servers

Several installs can be saved at once (an operator with more than one box). Each is a
`Connection` with a stable, random `id`; exactly one is **active** — the one the app is
talking to. Non-secret fields live in `ConnectionStore` (a list plus `activeId`); the
credentials are keyed by that id in the Keystore-backed `SecretStore`, so a switch never
leaks one install's session to another. Switching installs reloads the in-memory
credential (`AuthState`) and re-points the SSE socket (`LiveQueryStream`) at the new
install. A pre-multi-server install is adopted once via a legacy migration, so an
upgrade does not log the operator out.

### The shared live stream

`data/stream/LiveQueryStream.kt` owns **one** SSE connection to the active install,
shared by the Live Query Log, the Dashboard's live tiles and the notification service —
each holds a reference-counted `start()`/`stop()`. The server's stream carries every
resolved query, so one socket fanned out avoids multiplying connections for identical
data. It keeps a 200-entry, newest-first buffer seeded from the server's `history`
snapshot, and reconnects with backoff on a drop (stopping on a non-retryable close such
as a revoked session).

### Notes that shape the design

- **The admin path is random per install**, and the panel port is operator-chosen.
  The Retrofit base URL cannot be a constant; every call supplies an absolute URL
  resolved from the stored connection.
- **Installs are self-signed HTTPS.** The app uses trust-on-first-use, not a pinned
  public CA and not "trust everything". See `core/security/TlsTrust.kt`.
- **The v3 token is an idle-windowed `digitdns_session_` bearer.** A 401 is handled by
  `core/network/AuthRetryInterceptor.kt`, which asks `SessionRefresher` for a silent
  re-mint from the saved credentials and retries the call once; only when there is no
  saved pair does the app mark the session expired and route back to login. The master
  API key is the alternative credential.
- **Docs and the OpenAPI spec are bind-gated.** The app's calls must originate from
  a network the server's `api_bind`/`api_bind_list` admits, or the operator enables
  it; surface the 403 clearly in-app.
- **Push is SSE-based for v1.** A foreground service holds `/api/v3/events/stream`
  and raises local notifications — no FCM, no Google dependency, no server change.
  FCM can be layered on later behind the same notification surface.
- **English-only**, matching the panel.

### Theme parity

`ui/theme/PanelColors.kt` keeps both the `[data-theme="dark"]` and `[data-theme="light"]`
token blocks from `web/css/style.css` verbatim, because a panel token means
different things per theme (fills deepen as the number rises, text shades lighten,
and the two tiers invert in opposite directions). `ColorTokensTest` pins the ported
values against the CSS literals so a transposed hex digit fails the build.

The app follows the OS theme by default, with a three-way override (**Dark / Light /
Follow system**) in **Settings → App**, persisted in `data/AppPreferences.kt`.

### Dashboard parity

The Dashboard reproduces the panel's card grid: Query Rate, RAM Cache, SNI Proxy, CPU,
RAM and Live Traffic come straight from `/api/v3/stats`; a Compose-Canvas sparkline
charts the rolling query rate. Two cards the panel renders from per-upstream data —
Upstream Servers and Resolver Latency — read the read-only `GET /api/v3/upstreams/stats`
endpoint (added alongside the app so racers get real per-upstream latency) and the
`latency`/`latency_uncached` snapshots. The page polls every three seconds and honours
pull-to-refresh; **Settings → App** lets each card be hidden, and reordered by dragging
its handle, persisted in `AppPreferences`.

The card set matches the live panel exactly: the two live-only tiles the panel does not
ship (Live Queries and Blocked) are absent here too. The Upstream Servers tile carries the
panel's two write actions inline — add an upstream and remove one by swiping the row (a
confirmation follows) — via `POST /api/v3/upstreams/add` and `POST /api/v3/upstreams/delete`;
every other tile is read-only. Each upstream's latency number is coloured by the same
green/amber/red thresholds as its status dot. Each card header shows the panel's own icon
(`ui/components/PanelIcons.kt`), and the Resolver Latency colours track **p50 only**
(≥60 ms red, ≥25 ms amber, else green), leaving p95/p99/max neutral, as the panel does.

### Certificates, Connect and maintenance

- **Let's Encrypt Certificates** reads the on-disk certificate for each purpose from
  `GET /api/v3/config` (`dot_cert`/`panel_cert`/`subscription_cert`), shows its validity
  window, and relabels the action **Renew** when the server reports it expired. Switching
  the purpose chip re-seeds the domain field from that purpose's configured hostname. A
  renewal sends `force: true` so the server skips its cached-certificate fast path and
  places a fresh ACME order; the idle run card and last-run subtitle are hidden.
- **Connect** and **Diagnostics** take the Windows/console server IP from
  `config.server.public_ip` (falling back to the domain for the DNS address), so the
  guide never prints an empty or loopback address. Diagnostics runs on demand from a
  header **Run** pill; its **Servers** list shows each server's latest latency inline
  after a run, and servers delete by swipe.
- **About & Legal** carries the server's System card and the Update card (check/apply),
  relocated from the retired Updates page; the cache-flush card lives on **REST API
  Gateway**. Cache flushes report with a toast (snackbar) using the panel's own wording.
- **Security** shows the API key and admin path read-only with copy icons (regeneration
  stays on the REST API page) beside the 2FA enrolment flow, whose button reads
  **Start Enrolment**.
- **Settings&nbsp;&amp;&nbsp;SSL** presents its sections as a chip row with the panel's
  per-section icon beside each label.

The connection indicator is shared (`ui/components/ConnectionStatusBadge.kt`) and reads
**Online / Offline / Connecting / Reconnecting / Sign in again** everywhere; per-row
routing pills are Title Case (`Proxy`, `Blocked`, `Paused`).

### Client list freshness

The Clients list refreshes on every return to the screen (`LifecycleResumeEffect`), so a
client created, renamed or deleted in the detail screen never leaves a stale row behind;
a failed background refresh shows its error above the still-useful rows rather than
replacing them.
