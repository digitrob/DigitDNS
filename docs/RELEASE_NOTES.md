# DigitDNS v5.9.5 — working widgets and a lighter dashboard

This patch makes the Android home-screen widgets real: they can be added from
the widget picker, they refresh from the daemon, and the filtering toggle writes
an actual filtering change. It also trims the dashboard's once-a-second paint
and removes two small server-side bottlenecks.

No database schema migration is required. Existing settings, clients,
blocklists, certificates, relay agents, egress options and stored query history
are preserved. Upstream selection defaults are unchanged: Racing still maps to
Hedged.

## Android widgets you can actually use

- **Both providers are registered with the system.** The status and filtering
  toggle widgets finally appear in the Android widget picker instead of being
  inert code in the app.
- **The status widget follows the daemon.** It reads authenticated status when
  Android asks for an update, and the app mirrors its existing dashboard polls
  and settings screens into the widget so the numbers stay current.
- **The toggle really toggles filtering.** It reads canonical filtering state
  first, flips only `enabled`, and preserves the saved block mode, TTL,
  allowlist and custom rules. A quick settings tile and the Ad Blocking screen
  share the same state path.
- **Unavailable is honest.** When the app is signed out or the daemon cannot be
  reached, the widget shows an unavailable state instead of a stale on/off
  claim. Clickable areas also carry accessibility labels.

## A calmer dashboard tick

- **Stat tiles reuse their DOM nodes.** The one-second renderer caches element
  references and revalidates them with `isConnected`, so a reordered dashboard
  cannot leave a writer painting a detached node.
- **Counters share one number formatter.** A single locale-aware
  `Intl.NumberFormat` replaces a fresh formatter on every `toLocaleString()`
  call. Unchanged text, classes and tooltips are no longer rewritten each tick.
- The minified dashboard asset and its Brotli sidecar were regenerated from the
  updated source.

## Smaller server-side bottlenecks

- **Aggregate query totals no longer pin every write to one shard.** They are
  spread round-robin across the existing shards; flush and rollover still merge
  every shard, so no count is lost. The local benchmark fell from about 290 ns
  to 138 ns per total record, with the detail path unchanged.
- **Vetted relay targets are dialled in parallel.** Every candidate has already
  passed the target guard, so this does not widen the SSRF surface. The first
  successful connection wins, losers are cancelled and closed, and the reported
  error stays deterministic when all attempts fail.

## Android

The companion carries the matching `android-v5.9.5` version line. The workflow's
debug APKs are attached to this release alongside the standard signed APKs for
arm64-v8a, armeabi-v7a, x86_64 and universal devices.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.5/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.5
docker pull ghcr.io/digitrob/digitdns-agent:5.9.5
docker pull robmeister/digitdns:5.9.5
docker pull robmeister/digitdns-agent:5.9.5
```
