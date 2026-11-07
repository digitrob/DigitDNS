# DigitDNS v5.2.7 — web live log stays connected

**A web dashboard patch.** The live query log used to stop collecting whenever
the Log tab was not focused, so browsing in another tab left the log empty. The
stream is now opened once at dashboard boot and kept alive while the page is
visible; queries are buffered while the Log tab is hidden and appear when the
operator returns.

- The live log stays connected on Home, Clients, Agents and every other panel
  screen. No tab switch or hidden-tab stop drops queries.
- A daemon restart wipes the in-memory web session. A 401 from the stats poll or
  the SSE ticket now raises the login prompt instead of leaving stale tiles and a
  dead stream. Logging back in reconnects automatically.
- Stream startup is single-flight and generation-guarded: a visibility change, a
  manual refresh or a sign-out cannot leave two EventSources or an orphaned
  stream.
- Android behaviour is unchanged; its live log already worked.

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
docker pull ghcr.io/digitrob/digitdns:5.2.7
docker pull robmeister/digitdns:5.2.7
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.7-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.7` tag.
