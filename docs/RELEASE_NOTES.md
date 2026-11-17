# DigitDNS v5.9.1 — Force picks the resolver the panel calls fastest

A resolver-selection fix. No database schema migration is required; existing
settings, clients, blocklists, certificates, relay agents, egress options and
stored query history are preserved.

## What was wrong

Adaptive ranked resolvers by two different numbers. The card showed the live
p50 once a resolver had three live exchanges, while the selector used the
latest three-sample probe. On a pool whose top resolvers are within a
millisecond of each other, Force could therefore choose a row whose displayed
latency was *worse* than the current one, and each click re-probed and
re-ranked, so the highlight appeared to move only after several attempts.

## What changed

- **One number for the card and the choice.** While the strategy is Adaptive,
  the resolver list, its sort order, the row badge and the selection all use the
  same warm-probe median. Other strategies keep the live p50.
- **A stable winner across clicks.** Each resolver keeps its last five
  successful probe medians and Adaptive ranks on their median, so a difference
  inside the probe's own noise no longer moves the choice. A resolver that is
  genuinely faster still wins on the first click.
- **The card explains a kept choice.** Force applies a small noise floor and
  reports it, e.g. `kept 9.9.9.9:53; tls://dot.sb:853 is within measurement
  noise (8.0 vs 8.1 ms)`. That note is now shown under “Last run …” on web and
  Android instead of being sent and discarded.
- **Rows distinguish `FASTEST` from `IN USE`.** They are usually the same row;
  when a run deliberately keeps the current resolver, both markers are shown
  rather than calling the chosen row the fastest.
- **The numbers refresh in the same round trip.** The Force response now carries
  the post-benchmark resolver stats; the panel used to repaint from its previous
  `/api/stats` payload, which is cached for a second.
- **A successful benchmark probe closes an open circuit**, so a resolver that
  only the benchmark is reaching is no longer excluded from the Adaptive choice
  for the whole cooldown.
- **The resolver card's strategy and transport chips wrap** in the narrow
  column instead of shrinking under their own labels.

## Upgrade

In-place upgrades from the dashboard are supported. The Android app is a
side-loaded companion and keeps its own version line (`android-v5.9.1`).

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.9.1/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.9.1
docker pull ghcr.io/digitrob/digitdns-agent:5.9.1
docker pull robmeister/digitdns:5.9.1
docker pull robmeister/digitdns-agent:5.9.1
```
