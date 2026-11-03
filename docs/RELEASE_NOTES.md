# DigitDNS v4.0.1

A focused relay-agent patch. Routing, access control and relay behaviour are
unchanged; this release makes the agent log readable when the box is exposed to
the public internet.

## What was wrong

A public agent is found by scanners quickly. Requests that name the agent's IP
instead of a domain cannot be relayed — there is no destination hostname to
forward to — so the agent correctly refused them, but wrote one log line for
every attempt. An active probe loop could produce thousands of identical
lines and bury the relay traffic that matters.

## What changed

- **Refusal lines are throttled.** The first refusal for a source and reason is
  logged immediately. Repeats are folded into at most one line per minute with
  a `N similar events suppressed` count.
- **Bare-IP requests have their own counter.** They are reported as `bare-IP`
  in the 60-second summary instead of mixing with host-allowlist refusals.
- **The throttle map is bounded**, so a scanner cannot grow agent memory by
  rotating source addresses.
- Domain relaying, client checks, heartbeats, catalogue sync and traffic
  accounting are untouched.

The hub in this install is intentionally running with `allow_all: true`. This
patch does not change that; it only stops the resulting probe traffic from
filling the log. If the agent should be private, switch the hub Access mode to
whitelist and register the client IPs/CIDRs separately.

## Upgrade

- **Agent Docker:** pull `ghcr.io/digitrob/digitdns-agent:4.0.1` and recreate the
  container. Keep `/data/agent-state.json`.
- **Agent binary:** replace `digitdns-agent` and restart the unit.
- **Hub:** the patch is shipped alongside the hub binaries; replace the binary or
  pull `ghcr.io/digitrob/digitdns:4.0.1`.

No database migration and no configuration change. The Android app remains
4.0.0 and is compatible.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `checksums.txt`, `checksums.sig`

Full list of changes: [docs/CHANGELOG.md](CHANGELOG.md).
