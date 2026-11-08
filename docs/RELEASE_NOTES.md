# DigitDNS v5.3.1 — compact Settings top row

**Settings uses its width instead of stacking a full-width 2FA card.**

- **Server Location** is now a one-line card: the flag, country and GeoIP
  database date share a row, and the card is roughly half its previous height.
- **Two-Factor Authentication** sits directly underneath it in a tight left
  column; **Server Update** owns the right column. The old full-width 2FA row is
  gone, and the Hidden Admin Path / Administrator Credentials pair moves up.
- The layout is identical on **systemd and Docker**. The container image/binary
  selector simply makes the update card taller; it no longer decides where 2FA
  can be placed.

No resolver, API, database or Android behaviour changes. The Android APK is
rebuilt with the version bump only.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.3.1
docker pull ghcr.io/digitrob/digitdns-agent:5.3.1
docker pull robmeister/digitdns:5.3.1
docker pull robmeister/digitdns-agent:5.3.1
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.3.1/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.3.1-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.3.1` tag.
