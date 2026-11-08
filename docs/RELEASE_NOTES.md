# DigitDNS v5.3.2 — balanced Settings layout

**Settings cards now share their space instead of leaving gaps.**

- **Server Location** grows to fill the left column, so it ends level with
  **Server Update** on both systemd and Docker. Its title uses the same padding
  and top alignment as the update card, so the two headings sit on one line.
- The container image/binary selector still only changes the update card's
  height; the Location card absorbs the difference. There is no blank gap
  before Hidden Admin Path and no stretched, empty-looking card on systemd.
- On panels **1400 px and wider**, **Subscription Portal** and the **SSL / TLS
  Manager** share a row. The TLS card stretches to the taller card's height, and
  its field rows reflow to two columns with full-width action buttons instead of
  squeezing three controls into a half-width card.
- Below 1400 px both cards keep the normal full-width stacked layout, so narrow
  windows and mobile are unchanged.

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
docker pull ghcr.io/digitrob/digitdns:5.3.2
docker pull ghcr.io/digitrob/digitdns-agent:5.3.2
docker pull robmeister/digitdns:5.3.2
docker pull robmeister/digitdns-agent:5.3.2
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.3.2/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.3.2-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.3.2` tag.
