# DigitDNS v5.2.6 — reliability boundaries and responsive agent control

**A hardening and responsiveness release.** Request paths now contain panics,
the upstream breaker probes a recovered resolver instead of releasing it whole,
and the hub↔agent control plane no longer waits a minute for changes.

## Reliability

- **Panic containment.** The DNS handler, SNI and agent connection handlers, and
  upstream exchanges recover, log the stack and increment `panics_total`. A
  malformed request costs one SERVFAIL or one dropped connection, not the daemon.
- **Half-open upstream breaker.** After the cooldown exactly one probe is allowed;
  concurrent queries keep skipping the resolver until it answers. Recovery is
  jittered, logged, and exposed as `circuit_state`
  (`closed`/`open`/`half-open`).
- **Malformed-probe penalty box.** A source that produces repeated malformed
  handshakes is dropped for a minute before a handshake read or relay slot is
  spent. Drops appear as `relays_refused_flood`.
- **Agent sync jitter and backoff.** A hub restart no longer makes every agent
  reconnect on the same tick.
- **Faster failover.** The agent online window is 90 seconds (was two minutes),
  and agent-backed DNS answers use a 30-second TTL (the hub's own egress keeps 60).

## Performance and responsiveness

- **Gzip-negotiated cached JSON.** `/api/stats`, `/api/v3/stats` and the combined
  history document compress once per cache fill; the live stats payload drops from
  roughly 6 KB to under 1 KB on the wire.
- **The live query stream opens only on the Live Log tab.** Home uses the 1 Hz
  stats poll and no longer parses every query event.
- **Hub↔agent catalogue long-poll.** Catalogue responses carry an `ETag`; an
  unchanged allowlist answers `304` without recomputing hosts or clients. Agents
  long-poll with a 25-second wait, so a rule or client change normally arrives in
  under a second, with a five-minute full fetch as the safety net.
- **Incremental agent routing.** A heartbeat updates one agent in the live router
  instead of reloading every agent from the database.
- **Agent relay path.** Target hosts use a bounded 60-second DNS cache, multiple
  addresses are dialled with a 200 ms stagger instead of a 10-second wait on the
  first, both sockets get `TCP_NODELAY`/keepalive, and the listener has a 2,048
  connection cap and a 5-second handshake timeout.

No database migration. Older agents keep working against the new hub (they simply
do not use the long-poll or revision fields), and new agents fall back to the
periodic catalogue pull when the hub is older.

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
docker pull ghcr.io/digitrob/digitdns:5.2.6
docker pull robmeister/digitdns:5.2.6
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.6-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.6` tag.
