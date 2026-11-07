# DigitDNS v5.2.10 — no more “update in progress” that never ends

**The web updater can no longer spin forever.** v5.2.9 made Docker installs
refuse the binary-swap path and released a stale apply lock on the next click.
Two paths could still leave the progress modal waiting:

- `GET /api/update/status` returned a `Running` flag forever if the apply
  goroutine never reached its error handler. The endpoint now treats an apply
  older than six minutes as stale, clears it, and reports
  “the previous update stalled; check the service log and retry”.
- After a restart the in-memory progress is idle. If the daemon came back
  before a poll observed `restarting`, the modal polled `200 OK`
  `{running:false}` every 1.5 s with an empty message. It now treats that as
  finished and reloads.

**Docker still uses the image path.** Container installs report
`supported: false` and show:

```bash
docker compose pull && docker compose up -d
```

`POST /api/update/apply` is refused with the same command. systemd installs keep
the signed binary download, checksum/signature verification, pre-update backup
and restart path.

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
docker pull ghcr.io/digitrob/digitdns:5.2.10
docker pull robmeister/digitdns:5.2.10
```

systemd installs can still use the dashboard updater or:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.2.10/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.10-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.10` tag.
