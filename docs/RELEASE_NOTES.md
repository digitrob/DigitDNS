# DigitDNS v5.5.0 — client transports, DNSCrypt and mTLS

**One place for every client-facing transport, with independent switches,
token authentication, DNSCrypt and mTLS, plus a faster and steadier web and
Android panel.**

Existing settings, clients, blocklists, certificates and API keys are preserved.
No manual database migration is required.

## Client Transports

- **Plain DNS, DoT, DoH, DoQ, DoH3, DNSCrypt and mTLS** now live in one card on
  web and Android. Each has a port, a copyable endpoint and a switch.
- **Plain DNS and DoH can be disabled independently.** The DoH switch gates only
  `/dns-query`; the subscriber portal keeps serving on the DoH port.
- **DoT has its own enable switch** and defaults to on, so existing installs keep
  serving port 853 after the upgrade.
- **DoH3 stays on** and shares the DoH port over UDP; its switch is read-only.
- **Transport edits autosave** on web and Android. Incomplete SNI-token or mTLS
  forms stay local until their required inputs are present.
- **Endpoints match their certificates:** Plain DNS and the DNSCrypt stamp use
  the public IP; DoT, DoH, DoH3 and DoQ use the DoH/DoT certificate domain.
  SNI-token modes continue to use the token hostname.

## Authentication and encryption

- **DNSCrypt server** on **UDP and TCP 5443**, with encrypted provider keys,
  a signed certificate, a copyable stamp and live listener status.
- **DoH bearer tokens:** `/dns-query` accepts `Authorization: Bearer <token>` as
  well as the historical `?token=` parameter.
- **DoT SNI-token authentication:** the ServerName becomes
  `<token>.<token domain>`, checked before any query is read.
- **mTLS for DoT, DoH, DoH3 and DoQ** from an operator-supplied client CA. DoT
  requires a verified certificate at the handshake; DoH and DoQ enforce
  verification on the DNS query path so the subscriber portal stays reachable.
- **Metrics:** `digitdns_dnscrypt_queries_total` and
  `digitdns_dnscrypt_errors_total`.

## DNS and panel behaviour

- **DNSSEC saves apply live.** The validator is swapped in the running handler
  and the cache is flushed, so `RUNNING` changes in the same request. The button
  is now **Save & Apply DNSSEC Settings**.
- **Web Client Transports no longer flickers** when a switch is moved. Status
  labels, endpoints, the stamp, token lists and the status badge update only
  when their value changes.
- **Android settings pages open with card skeletons** instead of a full-screen
  spinner or blank screen. Initial loads no longer run the pull-to-refresh arc.
- **Android transport switches stay put.** Every field is included in the save
  request, and status plus switch share a fixed right-aligned slot.
- **RAM cards lead with the instance's own allocation** on web and Android, with
  machine-wide server RAM and usage as secondary lines.
- **Public DNS provider presets:** the upstream add box searches a curated
  top-ten catalogue, grouped by provider with bundled logos and every supported
  transport; typing a custom resolver address still works. The same browser is
  on the Android dashboard's Upstream Resolvers card.
- **Android relay and service pickers** use full-width rows with larger flags
  and text, plus consistent check-circle/radio selection pictograms.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.0
docker pull ghcr.io/digitrob/digitdns-agent:5.5.0
docker pull robmeister/digitdns:5.5.0
docker pull robmeister/digitdns-agent:5.5.0
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.0/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.5.0-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.0` tag.
The app version is **5.5.0 (541)**.
