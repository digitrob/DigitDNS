# DigitDNS v5.11.4 — Force regressions and a supported container base

This is a focused follow-up to v5.11.3. It fixes the Adaptive **Force** regressions found in review, makes the Android card report the real outcome, and moves the shipped images off an end-of-life Alpine branch. No database schema migration is required.

## Dashboard (Web)

- **The Force control recovers.** The button returns to `Force` after a run, stays disabled while a timed-out benchmark is still running server-side, and lets the next poll settle the result instead of leaving the card stuck on `Benchmarking…`.
- **The card and the decision agree.** A forced run ranks on the latest successful probe, and the Force response now renders the resolver list by that same metric (`latency_is_fresh`). The row marked **FASTEST** is the row the run selected, even when the probe-window median points at a different resolver. Scheduled runs keep the window median and 12% hysteresis; the tooltip shows the latest probe and the window median when they differ.
- **Decision feedback stays honest.** Switched/kept notes still carry the concrete millisecond values, and the tooltip explains which metric produced the order.

## Android

- **No more green flash on failure.** A failed Force run now shows the server's message (or a connection message) next to the control; only a successful run flashes the success colour.
- **The card footer carries the failure**, so an API or network error is visible without leaving the dashboard.

## Toolchain and container

- **Alpine 3.24.** The hub and agent runtime images move from `alpine:3.20` (end of support 2026-04-01) to `alpine:3.24` (supported to 2028-06).
- **Go 1.27.4.** CI, the GitHub release binaries and the Docker builder now all use Go 1.27.4, the current stable line, instead of a mix of older 1.26 patches.

The Android companion moves to **android-v5.11.4**, version code **586**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.4/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.4
docker pull ghcr.io/digitrob/digitdns-agent:5.11.4
docker pull robmeister/digitdns:5.11.4
docker pull robmeister/digitdns-agent:5.11.4
```
