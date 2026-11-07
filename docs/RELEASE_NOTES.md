# DigitDNS v5.2.9 — Docker-aware updater and no stuck apply lock

**The web update button now tells the truth in Docker.** The old apply path
downloaded a release binary and swapped the running executable inside the
container. It never pulled the image, and the next container recreate reverted
the swap. Container installs now refuse that path and show the command that
actually updates the deployment:

```bash
docker compose pull && docker compose up -d
```

- Docker/Podman is detected via `/.dockerenv`, `/run/.containerenv`, the container
  cgroup, or `DIGITDNS_CONTAINER=1` (set by the shipped image).
- `/api/update/check` reports `supported: false` with the pull/recreate note;
  `/api/update/apply` is refused with the same command.
- systemd installs keep the signed binary download, checksum/signature
  verification, pre-update backup and restart path.

**Stuck “update in progress” fixed**

- A `Running` apply older than six minutes is treated as stale, so a new attempt
  is allowed instead of returning 409 forever.
- A failed restart signal now surfaces an error, and a daemon that has not exited
  15 seconds after the signal releases the lock with an actionable message.
- The update card shows the server's note, so the Docker instruction is visible
  where the operator checks for updates.

No database migration. No resolver, relay or agent behaviour changes.

## Upgrade

On Docker, use the image path (do not use the in-panel Update button):

```bash
docker compose pull
docker compose up -d
```

Or pull the images directly:

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
docker pull robmeister/digitdns:latest
docker pull robmeister/digitdns-agent:latest
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.9
docker pull robmeister/digitdns:5.2.9
```

systemd installs can still use the dashboard updater or:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.2.9/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.9-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.9` tag.
