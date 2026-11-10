# DigitDNS v5.4.2 — compact Android Transports screen

**A small Android patch: the Client Encrypted Transports screen now fits
without scrolling.**

This release is an Android UI patch. The resolver, API, database, transport
behaviour and server-side configuration are unchanged; the server artifacts are
rebuilt with the 5.4.2 version so both products share one release page.

## What changed

- **Wide displays:** DoH3 and DoQ sit **side by side** instead of stacking, so
  the whole Transports screen fits without scrolling.
- **Phones:** the stacked layout is much shorter:
  - the redundant “Client Encrypted Transports” card is replaced by one muted
    certificate line,
  - the listener status badge moved into each card header next to its switch,
  - the separate “Authentication” label is gone,
  - the DoQ IP-allowlist sources, counts and Public Mode warning are combined
    into a compact note,
  - the SNI-token list is capped at 88 dp and scrolls inside the card instead
    of growing it.
- The outer scroll remains only as a safety net for very short screens or a
  long SNI-token list. The normal IP-auth screen needs no scroll on a
  411 × 914 dp phone or a 1280 × 720 dp tablet.
- Removed four unused Android strings left over from the old layout.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.4.2
docker pull ghcr.io/digitrob/digitdns-agent:5.4.2
docker pull robmeister/digitdns:5.4.2
docker pull robmeister/digitdns-agent:5.4.2
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.4.2/scripts/install.sh | sudo bash
```

No database migration is required.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.4.2-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.4.2` tag.
