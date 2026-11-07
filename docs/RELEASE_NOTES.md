# DigitDNS v5.2.4 — SNI Refused counts real refusals only

**A patch for the SNI proxy card.** Public port 443 constantly receives
connections with invalid SNI/Host values. Those malformed probes were counted in
`relays_refused`, so a healthy proxy looked like it was refusing traffic all day.

- Malformed SNI/Host probes no longer increment `relays_refused`. They are
  counted separately as `relays_refused_malformed` and shown in the Refused
  tooltip as “malformed probes … (not counted)”.
- `relays_refused` now covers only actionable refusals: blocked targets,
  self-dials, spent quota, a full relay table and resolution failures.
- The card stays quiet on a public proxy port; no extra line was added.
- No database migration and no other behavior changes.

## Upgrade

The in-panel self-updater and the one-line installer use the same release.

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

Docker Hub mirror:

```bash
docker pull robmeister/digitdns:latest
docker pull robmeister/digitdns-agent:latest
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.4
docker pull robmeister/digitdns:5.2.4
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.4-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.4` tag.
