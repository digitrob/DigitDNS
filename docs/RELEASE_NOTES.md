# DigitDNS v5.2.12 — choose how a container updates

**Docker installs can now pick the update method** in **Settings → Server Update**.

- **Pull image (recommended)** — the default. The panel refuses the
  in-container swap and shows the command that updates the real deployment:

  ```bash
  docker compose pull && docker compose up -d
  ```

- **Patch binary in this container (temporary)** — re-enables the in-panel swap.
  The release binary is still downloaded, its SHA-256 checked against the signed
  `checksums.txt`, the data files backed up, and the daemon restarted. The
  difference is honest and visible: the image tag and digest do **not** change,
  and a container recreate restores the image's binary.

The choice is container-only. On a systemd install the selector is hidden, the
API rejects a change, and the updater ignores any stale value; the signed binary
path is unchanged.

**A badge tells the truth.** Beside the pre-update backups the daemon records the
container ID, the image's version and the swapped version. While a swapped binary
is running, the card shows **“Temporary binary update active — a container
recreate restores vX”**. A recreate changes the container ID, so the marker is
cleared and the card returns to the image state.

**Android** gets the same selector on the About page (container installs only),
the temporary badge, and a compact layout with the update controls first, so
they fit on one screen without scrolling.

No database migration. No resolver, relay or agent behaviour changes.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.12
docker pull ghcr.io/digitrob/digitdns-agent:5.2.12
docker pull robmeister/digitdns:5.2.12
docker pull robmeister/digitdns-agent:5.2.12
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.2.12/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.12-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.12` tag.
