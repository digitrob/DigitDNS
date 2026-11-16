# DigitDNS v5.8.18 — the row you clicked changes

A routing and reliability release on top of v5.8.17. No database schema migration
is required; existing settings, clients, blocklists, certificates, relay agents,
egress options and stored query history are preserved.

## Fixed

- **A `*.host` rule no longer makes the host itself look direct.** The scope is
  deliberately subdomains-only: `*.reflection.ai` does not match `reflection.ai`.
  The quick-action menu made that easy to miss. Opening it on the apex while only
  `*.reflection.ai` existed preselected `*.host`, so pressing **Proxy** wrote the
  same wildcard again and the row you clicked stayed direct. The menu now opens
  on the scope that still covers the clicked host: the bare rule, which the
  matcher indexes as both the exact host and a wildcard, so it covers the host
  and every subdomain.

  Choosing `*.host` explicitly still writes the subdomains-only rule, but the
  preview and the toast now say that the clicked host itself is unchanged.

- **The `exact` scope says what it does again.** v5.8.17 changed its hint to
  "this host only"; that was false. A plain rule matches the host and every
  subdomain, so the hint is back to **host + subdomains**.

- **Add moves a domain instead of leaving a second copy behind.** Direct is
  checked before Proxy in the matcher, so adding a name that was already in
  Direct left the Direct rule winning and the Add looked ignored. The Add box now
  removes the name and its own host wildcard from Direct, Blocked and the paused
  list, the same way the live-log quick action already did.

- **Force on the resolver card waits for a benchmark that is already running.**
  Clicking Force during a scheduled, startup or manual benchmark returned
  "A benchmark is already in progress" and did nothing. It now waits (up to 30
  seconds) for the in-flight run to finish and selects from the measurements that
  run was already taking, so one click still switches to the fastest healthy
  resolver. The scheduled interval keeps its short retry and is not held up by a
  manual run.

- **The Android quick-action menu uses the same scope rule.** A wildcard-only
  rule no longer preselects `*.host` on the host itself, and the menu says when a
  chosen scope leaves the clicked host unchanged.

## Changed

- **The container healthcheck is a TCP connect, not an HTTPS request.** The old
  probe used BusyBox `wget` over TLS. On some kernels (observed on Ubuntu 26.04 /
  7.0.0-43) every HTTPS probe leaked one task slot in the container's cgroup
  counter: `pids.current` grew by exactly one per probe while `cgroup.threads`
  stayed constant. At the default 30-second interval the systemd
  `DefaultTasksMax` of 1066 was reached in about nine hours; from then on every
  `runc exec` — including the healthcheck itself — failed with
  `procReady not received`, and a perfectly healthy resolver was reported
  unhealthy forever.

  The probe is now `nc -z` against the panel port: no TLS, no leak, and it works
  whether the panel serves HTTP or HTTPS. `/healthz` remains the endpoint for
  external HTTP monitoring. The loopback TLS handshake line the TCP probe would
  otherwise leave on an HTTPS panel is filtered; remote handshake errors are
  still logged.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.18` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.18/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.18
docker pull ghcr.io/digitrob/digitdns-agent:5.8.18
docker pull robmeister/digitdns:5.8.18
docker pull robmeister/digitdns-agent:5.8.18
```

If your own Compose file overrides the image healthcheck, replace the HTTPS
`wget` probe with:

```yaml
healthcheck:
  test: ["CMD-SHELL", "nc -z -w 2 127.0.0.1 $${DIGITDNS_HEALTH_PORT:-8080}"]
```
