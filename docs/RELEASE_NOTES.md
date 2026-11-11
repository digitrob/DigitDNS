# DigitDNS v5.5.5 — faster resolver and dashboard paths

A performance release on top of v5.5.4. No resolver, API or database schema
migration is required. Existing settings, clients, blocklists and certificates
are preserved.

## Resolver

- **Lock-free matcher reads.** The DNS query path now reads an immutable rule
  snapshot instead of taking an RWMutex read lock for every query. Rule rebuilds
  publish a new snapshot atomically, so a settings save no longer contends with
  live traffic.
- **Quota-refusal logging reuses the parsed question**, removing a second parse
  on that path.

Benchmark on the development machine:

- `BenchmarkMatchParallel` — ~19–20 ns/op, 0 allocs/op
- `BenchmarkMatchExactHit` — ~67 ns/op, 0 allocs/op

## Panel

- **Client list from the in-memory index.** The dashboard no longer decrypts
  every client record from the database on each poll; the service snapshots its
  existing index and adds the live traffic/query deltas. The response shape and
  order are unchanged.
- **Gzip response pooling.** The API gzip writer and wrapper are pooled and
  reset per request instead of allocated per response.
- **Login branding** now reads **SmartDNS · SNI Proxy · Ad Blocking**.

## Agents

- **Catalogue body cache.** The hub caches each agent's encoded catalogue for
  the current control-plane revision. A config change clears the cache; the key
  also includes the agent's country and effective IPv6 flag.

## API

- **V3 upstream stats cache.** `/api/v3/upstreams/stats` is memoised for one
  second, matching the dashboard's resolver-racer cadence. The response shape is
  unchanged.
- No DTO, auth, route or schema changes.

## Android

- App version **5.5.5 (546)**. No functional Android changes; the app already
  uses the same v3 endpoints and benefits from the faster backend.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.5
docker pull ghcr.io/digitrob/digitdns-agent:5.5.5
docker pull robmeister/digitdns:5.5.5
docker pull robmeister/digitdns-agent:5.5.5
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.5/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.5` tag.
