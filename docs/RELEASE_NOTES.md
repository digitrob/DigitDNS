# DigitDNS v5.4.0 — client-facing DoH3 and DoQ

**HTTP/3 and DNS-over-QUIC endpoints for clients, both opt-in.**

DigitDNS has always spoken DoQ and DoH3 upstream. This release turns the same
transports around: clients can now reach this resolver over **DoH3** and
**DoQ**, alongside the existing UDP/TCP, DoT and DoH listeners. Both are off by
default and use the DoH/DoT certificate, so an existing install behaves exactly
as before until an operator enables them.

## What is new

- **DoH3 on UDP 8443.** The same `https://host:8443/dns-query` URL as DoH,
  negotiated over QUIC. TCP DoH advertises it with `Alt-Svc`, so browsers and
  clients that support HTTP/3 upgrade automatically; everyone else stays on
  TCP DoH.
- **DoQ on UDP 853.** RFC 9250 framing (two-octet length prefix, transaction ID
  0, one query per stream), with a 1,024-connection cap, 128 streams per
  connection, a 60-second idle timeout and no 0-RTT.
- **DoQ authentication.** Use the existing client IP allowlist, or issue
  SNI-label tokens and connect with
  `quic://<token>.doq.example.com:853`. Token mode is standard-client
  compatible; it requires a certificate covering the token hostnames.
- **A real Settings card.** **Client Encrypted Transports** sits directly under
  the SSL/TLS Manager. It has DoH3 and DoQ switches, visible listener state,
  copyable endpoints, DoQ port/auth/token controls and one Save action. A bind
  failure is reported instead of being shown as a running endpoint.
- **Web and Android parity.** The Connect Guide and subscriber portal list the
  new endpoints. Android gains a **Settings → Transports** screen with the same
  switches, port, auth mode and token list; its Connect and Guide screens show
  DoH3/DoQ when enabled.
- **Metrics.** `digitdns_doh3_requests_total`,
  `digitdns_doq_queries_total` and `digitdns_doq_auth_failures_total` join the
  existing resolver and relay metrics.
- **Clearer SNI proxy cards.** Actionable refusals and unreadable scanner
  traffic are reported separately on web and Android, so a public 443 under
  scanning no longer looks like a failing relay.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.4.0
docker pull ghcr.io/digitrob/digitdns-agent:5.4.0
docker pull robmeister/digitdns:5.4.0
docker pull robmeister/digitdns-agent:5.4.0
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.4.0/scripts/install.sh | sudo bash
```

## Enabling the new transports

1. Make sure the DoH/DoT certificate is valid for the hostname clients use.
2. Open **UDP 853** and **UDP 8443** in the firewall.
3. Open **Settings → Client Encrypted Transports**, enable DoH3 and/or DoQ,
   choose the DoQ authentication mode, then **Save Transports**.

For DoQ SNI-token mode, supply a certificate covering
`*.doq.example.com` (or each token hostname). The built-in ACME client uses
HTTP-01 and cannot issue wildcards; issue that certificate separately.

Android Private DNS remains **DoT**. DoQ is for clients and routers that
support RFC 9250.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.4.0-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.4.0` tag.
