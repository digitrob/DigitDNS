# DigitDNS v5.4.1 — transport hardening and allowlist clarity

**A patch release for the 5.4 line: safer live transport changes, QUIC limits,
and a DoQ access screen that explains itself.**

v5.4.1 keeps the feature set of 5.4.0 and hardens the parts that were easiest to
misread: applying a transport change, seeing whether its listener is really up,
and understanding which IPs the DoQ “IP allowlist” mode accepts.

## Live transport changes

- **DoH3 applies immediately** when its switch moves. **DoQ keeps Save DoQ**
  because its port, auth mode, token domain and token list must be valid
  together.
- The server now **applies before it persists**. A bind or certificate failure
  rolls the listeners back to the previous state, so the saved switch can never
  get ahead of what is actually serving.
- The card keeps the failure reason in `last_error` after the toast is gone,
  including a listener that dies later.
- DoH3 reports **listening only after the HTTP/3 serve loop has started**, and a
  failed start is shown as “enabled — not listening”, not as a healthy endpoint.

## QUIC hardening

- **Per-source handshake rate limiting** runs before TLS: 60 burst and 2 new
  handshakes per second per IP for both DoH3 and DoQ, with a bounded source map.
- DoH3 gains a **1,024 live-connection cap** (matching DoQ) and capped HTTP/3
  and DoQ flow-control windows.
- New Prometheus series:
  - `digitdns_doh3_connections`, `digitdns_doh3_connections_total`
  - `digitdns_doq_connections`, `digitdns_doq_connections_total`
  - `digitdns_doq_protocol_errors_total`
  - `digitdns_quic_handshake_rejected_total{transport="doh3"|"doq"}`

## DoQ access control is explicit

- The DoQ card now says where IP auth gets its addresses: **Clients →
  Allowed IPs** and **Settings → Access → Server-wide Resolver Allowlist**.
- It shows live counts for client accounts, client IP/CIDR entries and resolver
  ranges.
- It warns when **Public Mode (`allow_all`)** makes the allowlist irrelevant,
  and when whitelist mode has no entries and DoQ will refuse every source.
- Android Transports has the same explanation, counts and warnings.

## Resolver health

- A background health check re-probes every upstream every **five minutes** and
  logs one warning when an upstream is degraded plus one recovery line.
- Degraded means an open circuit, a failed latest probe, or a recent live error
  rate of **10% or more over at least 20 queries**.
- Fallback behaviour is unchanged: a failed fastest upstream still hands the
  query to the next eligible one.

## Android

- DoH3 auto-save and staged **Save DoQ**, with applied-versus-staged state and
  server errors shown on the screen.
- The Save DoQ button now sits beside the firewall note, removing a row from the
  DoQ card.
- Version **5.4.1 (538)**.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.4.1
docker pull ghcr.io/digitrob/digitdns-agent:5.4.1
docker pull robmeister/digitdns:5.4.1
docker pull robmeister/digitdns-agent:5.4.1
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.4.1/scripts/install.sh | sudo bash
```

No database migration is required. Existing DoH3/DoQ settings and DoQ tokens
are preserved.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.4.1-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.4.1` tag.
