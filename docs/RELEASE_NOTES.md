# DigitDNS v5.13.0 — One Resolver widget, a Domains toolbar that stays put, and faster query paths

A performance-and-polish release. The two old home-screen widgets become one Resolver widget with a real resolver list and per-widget appearance settings; the Domains search field stops moving when domains are selected; and the resolver's hot paths lose a formatting allocation, a shard-wide eviction scan, a duplicate cache write and a serialised log write. Two correctness bugs that produced retransmits and a data race are fixed. No database schema migration is required.

## One Resolver widget

The Status and Toggle widgets are replaced by a single **Resolver** widget:

- Lists the pool with the provider mark, the transport **inline beside the name** (DoH, DoT, DoQ, DNSCrypt, plain), the measured latency and a status dot. The resolver currently in use carries a **FASTEST** badge and a wash; transports excluded by policy are dimmed.
- Header: last update time, an animated refresh (spinner while the fetch runs) and a gear that opens the appearance editor.
- Footer: a **Filtering ON/OFF** pill that toggles the server and an app launcher.
- **Four resolvers fit at the default size.** The row, header and footer metrics were compacted and the widget's minimum height raised to three launcher rows, so a 3×3 placement shows four rows without the last one colliding with the footer.
- **Widget Settings** now sit under **Settings → App → Widget Settings**, not nested inside the App card. Theme (app/dark/grey/light), background opacity and text size are editable per placed widget; the widget's gear and the launcher configure dialog open the same editor and write the same preferences, and every change repaints in place. Each slider has a short tip describing exactly what it changes.

Existing placements of the old Status/Toggle widgets will disappear on upgrade; add the Resolver widget from the picker.

The launcher no longer answers with an empty box: a toolbar spacer used a bare `<View>`, which RemoteViews refuses to inflate. The layout now uses only RemoteView-allowed classes and an explicit ProgressBar style.

## The Domains toolbar stops moving

The selection count was a `min-width: 5rem` floor. "No domains selected" is about 7rem at 11px, so it widened the slot; the first tick shortened the label to "1 selected", the slot collapsed to its floor, and the flex search field grew by the difference — moving the field, the sort control and every bulk button. The count now occupies a fixed, right-aligned slot with tabular digits, so the search box keeps its width at every breakpoint.

## Faster, steadier resolver

- **Cache-hit work is smaller.** With no live-log overrides, custom records or `$dnsrewrite` rules — the normal state — the matcher returns without walking the name's labels, and custom-record lookup reuses the already-normalised name. Single-flight keys are comparable structs, not formatted strings.
- **Eviction no longer scans the shard.** At capacity, each insert sampled sixteen entries instead of walking all of them (twice) under the shard write lock.
- **Followers stop duplicating work.** Only the single-flight leader writes the shared answer to the cache; waiters no longer re-copy, clamp and normalise it.
- **Lossy links do not serialise the pool.** Latency-order invalidation is throttled to a few times a second, and circuit-transition log lines are written after the per-upstream lock is released. Plain TCP upstreams and the truncation fallback reuse idle connections.
- **The live log no longer encodes on the resolver goroutine.** Query items are queued and marshalled once on a dedicated encoder goroutine. The aggregate's hour key is cached and the shard hash no longer concatenates IP + domain.
- **A larger UDP receive buffer** (4 MB, Linux best-effort) absorbs query bursts instead of dropping them and waiting for the client's retransmit.
- **Outbound fetches reuse connections.** Blocklist sources, AdGuard imports, icon search and RDAP share one keep-alive HTTP/2 client; an abandoned AdGuard preview cancels its download.

## Correctness fixes

- **Loopback rate-limit exemption on plain DNS.** UDP/TCP/DoT passed `host:port` where the limiter expected a bare IP, so the exemption never applied and local health checks, the panel's resolver test and the TUI could be refused once loopback passed the limit.
- **Truncated answers cannot be cached.** The client path already refused TC=1, but the background refresh and prefetch writers reached the cache directly; a partial UDP fallback could be served to every client for a full TTL.
- **The public IPv6 address is atomic.** Saving the IPv6 card while queries were served was a genuine data race; it is now published through an atomic pointer and loaded once per query.
- **A panicking single-flight leader** broadcasts a failure to its waiters instead of leaving them blocked forever.

## Dashboard polish

The three end-of-body scripts are deferred, the icon sweep is scoped to the panel it just rendered, the AdGuard preview filter is debounced, the system-log and live-log fallback timers stop when their tab is hidden, and dead CSS/JS was removed.

## Verification

`go vet`, the full Go suite, the race detector on the resolver/cache/upstream/service packages, the dashboard regression tests, Android unit tests and the debug build pass. Web assets were rebuilt and the minified-hash tests pass. No database schema migration is required.

The Android companion moves to **android-v5.13.0**, version code **594**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.13.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.13.0
docker pull ghcr.io/digitrob/digitdns-agent:5.13.0
docker pull robmeister/digitdns:5.13.0
docker pull robmeister/digitdns-agent:5.13.0
```
