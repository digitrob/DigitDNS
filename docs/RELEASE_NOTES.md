# DigitDNS v5.11.5 — Faster DNS failover and immediate Force

This release focuses on the time a website waits when a DNS resolver is slow or silent, and on making **Force** mean “switch now”. It also keeps encrypted upstream connections warm and lets bootstrap DNS resolve their hostnames without relying on the system resolver. No database schema migration is required.

## DNS failover

- **Each resolver gets its own turn.** The fallback walk gives every resolver an 800 ms slice of the 2 s budget instead of letting the first one consume all of it. The last resolver still gets the remainder.
- **A failed resolver is demoted immediately.** One transport timeout, or a lost hedge, moves it behind the healthy resolvers and excludes it from selection until it answers again. A success resets it.
- **Adaptive hedging.** The measured fastest resolver gets a 100 ms head start. If it has not answered, the next healthy resolver is asked in parallel; the first usable answer wins and the loser is cancelled. A fast first resolver sends no extra query.
- **Result:** a slow first resolver answers in about 100 ms, and a silent one fails over in about 100 ms instead of waiting ~2 s and failing.

## Force

The **Force** control now switches from the latest measurements immediately. It no longer waits for an in-flight scheduled benchmark and no longer runs a full three-sample benchmark on click. The control and tooltip read “Switch…” to match. Only a pool that has never been measured takes one quick probe.

## Encrypted upstreams

- **Warm connections.** Active DoH/DoT/DoQ/DoH3 connections are refreshed every 20 s, so the first lookup after a quiet period does not pay a new handshake. The keep-warm queries are not recorded as probes or live samples.
- **Bootstrap DNS.** New `dns.bootstrap_dns` config key: plain DNS servers used only to resolve encrypted upstream hostnames. Set it when the server is its own system resolver to avoid a lookup loop. Empty keeps the OS resolver.

```json
{
  "dns": {
    "bootstrap_dns": ["1.1.1.1:53", "9.9.9.9:53"]
  }
}
```

## Tests

Regression coverage now includes per-attempt failover, demotion, Adaptive hedging, selection probation, immediate Force, bootstrap resolution, and keep-warm connection reuse. Full Go, dashboard and Node suites pass, and the behaviour was measured on the live Norwich instance.

The Android companion moves to **android-v5.11.5**, version code **587**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.5/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.5
docker pull ghcr.io/digitrob/digitdns-agent:5.11.5
docker pull robmeister/digitdns:5.11.5
docker pull robmeister/digitdns-agent:5.11.5
```
