# DigitDNS v5.3.3 — accurate SNI proxy drop counters

**Scanner noise no longer looks like relay failure.**

- The **web** and **Android** SNI proxy cards used to report
  `relays_refused + relays_unreadable` as “drops”. `relays_refused` includes
  flood attempts from a source already in the malformed-probe penalty box, so a
  single scanner could show tens of thousands of drops against a few hundred
  real relays.
- The cards now show **actionable refusals plus unreadable connections**. Flood
  attempts and malformed probes are excluded from the headline.
- The web card keeps the full breakdown in its tooltip: blocked, self-dial,
  quota, table full, resolve failure, flood attempts and malformed probes.
- Prometheus gains:
  - `digitdns_relays_refused_actionable_total`
  - `digitdns_relays_flood_total`

  `digitdns_relays_refused_total` keeps its existing total semantics.

No resolver, filtering, routing, database or Android networking behaviour
changes. The Android APK is rebuilt with the version bump and the corrected
dashboard card.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.3.3
docker pull ghcr.io/digitrob/digitdns-agent:5.3.3
docker pull robmeister/digitdns:5.3.3
docker pull robmeister/digitdns-agent:5.3.3
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.3.3/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.3.3-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.3.3` tag.
