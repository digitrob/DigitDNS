# DigitDNS v5.3.0 — faster cached queries, metrics docs and Android startup

**The resolver's cached path is leaner.** Entries that were stored already shaped
for their EDNS/DO/CD key no longer repeat that normalisation on every hit, and the
matcher/cache reuse the lowercase name the resolver already computed for logging.
`BenchmarkProcessCacheHit` dropped from **~932 ns to ~770 ns** per query, with the
same 296 B / 7 allocs. The wire-format cache was evaluated and deferred: profiling
put the message copy at ~5 % of a hit, not enough to justify rewriting every
transport.

**The dashboard is lighter and observable.**

- Favicons moved out of `index.html` into content-versioned images; the document
  is 228.6 KB → 219.8 KB and the icons are cached immutably.
- New `docs/METRICS.md` and an importable `docs/grafana/digitdns.json` covering
  queries, cache, latency, relay traffic and the agent control plane.
- `docs/API.md` documents `/api/metrics`, `/api/update/mode` and the rule-tester
  routes.
- Relay Agents on web and Android show the applied catalogue revision and age.
- The update card links to the matching GitHub release and can copy the
  `docker compose pull && docker compose up -d` command.
- Agent fleet counts behind `/api/metrics` are cached for five seconds.

**Android starts better and is easier to drive.**

- A `NavigationRail` replaces the bottom bar at 600 dp and above; phones keep the
  bottom bar.
- A Quick Settings tile toggles ad blocking through the same v3 filtering API as
  the Ad Blocking screen.
- Baseline and startup profiles are generated from the real startup path and
  compiled into release builds as `assets/dexopt/baseline.prof` /
  `baseline.profm`, with `androidx.profileinstaller` ensuring they are applied.
- Relay Agents shows **Applied** and its age; the About page keeps the update
  controls on the first screen.

**CI** pins cosign v2.5.2 and downloads it with retries, removing the transient
`Docker main` failure caused by the floating installer action.

No database migration. No resolver policy, relay or agent protocol changes.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.3.0
docker pull ghcr.io/digitrob/digitdns-agent:5.3.0
docker pull robmeister/digitdns:5.3.0
docker pull robmeister/digitdns-agent:5.3.0
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.3.0/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.3.0-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.3.0` tag.
