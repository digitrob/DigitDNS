# DigitDNS v5.2.5 — trustworthy upstream metrics, faster relay and stats

**The upstream card now reports what actually happened.** Latency is the median
of real transport round trips instead of the wall-clock time around the whole
call, startup probes no longer inflate live hits and errors, and the health dot
follows the recent error rate instead of a lifetime total.

- Latency is the median of the last 128 successful live exchanges; the tooltip
  carries p95, sample count and the recent error rate.
- An upstream that has not answered reports `measured: false` and renders `—`.
  The seeded 12 ms estimate is gone.
- Startup and benchmark probes are counted separately (`probe_hits`,
  `probe_errors`, `last_probe_ms`) and never trip the circuit breaker.
- REFUSED and SERVFAIL are policy answers: they prove reachability and stay
  distinct from transport failures.
- The dot means health; the latency badge keeps its own colour. A slow but
  healthy resolver stays green.

**Performance**

- Unmetered SNI relays use `TCPConn.ReadFrom` (splice) and still honour the idle
  timeout. Metered relays keep the buffered, per-1 MB accounting path.
- The relay buffer is now 64 KiB.
- Live stats and the combined history endpoint are cached per process (1 s and
  5 s), so several dashboards share one computation.

No database migration. The Android companion now reads the new `measured`,
`recent_error_rate` and probe fields and colours the dot by current health.

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
docker pull ghcr.io/digitrob/digitdns:5.2.5
docker pull robmeister/digitdns:5.2.5
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.5-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.5` tag.
