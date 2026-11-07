# DigitDNS v5.2.8 — web live log survives buffered SSE

**A web live-log reliability patch.** Some browsers or reverse proxies buffer an
EventSource connection even though it opens successfully, so the log page sat on
“Listening…” while Android — which connects directly — showed queries. The web
now falls back to the server's recent-log ring when SSE stays silent.

- Added authenticated `GET /api/stream/recent`, returning the last hundred queries
  from the same in-memory ring the SSE history uses.
- If the EventSource delivers no frame for three seconds, the web log starts
  polling that endpoint every two seconds and stops immediately when SSE resumes.
  Polling runs only while the Log tab is visible and skips an unchanged ring.
- The empty state now distinguishes **Connecting**, **Connected via polling**,
  **Listening** and **Paused** instead of always saying “Listening”.
- Pause/Resume now has a fixed 96 px box, so the search bar and filter row do not
  resize when the label changes between the two verbs.
- The filter compile log now reads `rules (exact + suffix) / unique domains`.
  A plain domain is stored twice by design: exact for the apex and suffix for its
  subdomains, so roughly two lookup entries per unique name is expected.

No database migration. No resolver, relay or agent behaviour changes.

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
docker pull ghcr.io/digitrob/digitdns:5.2.8
docker pull robmeister/digitdns:5.2.8
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.8-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.8` tag.
