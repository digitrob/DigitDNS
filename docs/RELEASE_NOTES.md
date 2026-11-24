# DigitDNS v5.11.3 — Adaptive Force check fixes

This release focuses on the Dashboard **Adaptive** resolution “Force” (Check now) flow: the control is now a single clickable button, forced ranking uses the freshest probe with a visible (0.1 ms) difference rule, and feedback is clearer and more reliable. No database schema migration is required.

## Dashboard (Web)

- **Single Force control:** the “Force” button is clickable as one control (icon + label) with proper focus styling and a `Benchmarking…` label while running.
- **Forced vs scheduled behavior:** Force ranks using the latest successful probe for that run’s decision; switches happen only when the visibly fastest resolver is lower by the displayed 0.1 ms granularity versus the current choice. Scheduled Adaptive checks continue to use the median-window logic to reduce flapping.
- **Better feedback:** toasts/notes reflect the server’s decision (switched/kept with ms values). The UI uses an epoch/timeout guard so stale responses and long-running benchmarks don’t produce confusing UI states; on timeout it warns that the server may still finish and the card updates on the next poll.
- **Aligned stats:** the Force endpoint returns post-benchmark upstream stats so the UI shows consistent numbers after a run.

## Android

- **Merged Force control:** “Check now” is a single tappable row (not split icon/text controls).
- **Clearer outcomes:** success is indicated only after a successful run; API errors are surfaced to the user; the card note shows the server’s decision text when provided.

The Android companion moves to **android-v5.11.3**, version code **585**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.3/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.3
docker pull ghcr.io/digitrob/digitdns-agent:5.11.3
docker pull robmeister/digitdns:5.11.3
docker pull robmeister/digitdns-agent:5.11.3
```
