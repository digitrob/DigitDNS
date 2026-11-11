# DigitDNS v5.5.7 — adaptive upstreams and transport chips

A feature release on top of v5.5.6. No database schema migration is required;
existing settings, clients, blocklists and certificates are preserved.

## Resolver strategy

- **Adaptive (fastest)** is now the default strategy.
- Every **5 min / 15 min / 1 hr / 6 hr / 24 hr** the pool is benchmarked, the
  fastest healthy resolver is selected with **12% hysteresis**, and the choice is
  persisted with its last-run time and latency.
- **Force** runs the same benchmark and selection immediately.
- An empty saved strategy now reads as Adaptive.
- **Pool default** is removed. **Fastest wins** is hidden; legacy `fastest`
  values normalise to Hedged.

## Transport chips

- **Plain, DoT, DoH, DoH3, DoQ and DNSCrypt** are independent toggles.
- Multiple protocols can be enabled together.
- The resolver pool only uses upstreams whose protocol is enabled.
- At least one protocol stays enabled.

## Resolver card

- Strategy, interval and transport controls are compact chips.
- Provider favicons appear in the resolver list and the Single Server dropdown.
- The resolver currently used by Adaptive is highlighted.
- **Pool / Single Server** switch:
  - Pool uses the configured strategy and transport chips.
  - Single Server uses one resolver and greys out strategy, interval, Force and
    transport chips.
  - The last selected single server is remembered.
- Last-run text reserves two lines for long server names.

## Android

- The same resolver controls, compact chips and abbreviated intervals.
- Each chip group stays on one line and scrolls horizontally when needed.
- Biometric prompt is raised automatically when the lock screen opens.
- Returning from a long background period reports **Online** while the SSE socket
  refreshes, instead of flashing **Connecting**.
- App version **5.5.7 (548)**.

## API

- Resolution settings add `transports`, `adaptive_interval_minutes`,
  `adaptive_last_run`, `adaptive_current`, `adaptive_last_latency_ms` and
  `adaptive_last_note`.
- New forced-run endpoint:
  `POST /api/v3/settings/resolution/run-adaptive`.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.7
docker pull ghcr.io/digitrob/digitdns-agent:5.5.7
docker pull robmeister/digitdns:5.5.7
docker pull robmeister/digitdns-agent:5.5.7
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.7/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.7` tag.
