# DigitDNS v5.8.19 — the healthcheck can no longer wedge the container

A small reliability release on top of v5.8.18. No database schema migration is
required; existing settings, clients, blocklists, certificates, relay agents,
egress options and stored query history are preserved.

## Why v5.8.18's TCP probe did not fix it

v5.8.18 replaced the HTTPS `wget` probe with a TCP connect (`nc -z`) to the
panel port. That was not enough. On some kernels Docker's healthcheck **exec**
into this container leaks one cgroup task slot per probe, whatever the command
is:

- `pids.current` grew by exactly one per scheduled probe while
  `cgroup.threads` stayed constant — the same rate as the old HTTPS probe.
- A throwaway container running the identical `nc -z` probe leaked nothing, so
  the command was never the cause; the healthcheck exec into the running
  DigitDNS container is.
- At the systemd default of 1066 tasks the container reached the cap in about
  nine hours. From then on every `runc exec` — including the probe itself —
  failed with `procReady not received`. Docker never restarts a container just
  because it is unhealthy, so a perfectly healthy resolver was reported
  unhealthy forever.

## Changed

- **The in-image `HEALTHCHECK` is gone.** There is no exec healthcheck left to
  leak a task slot. The panel's public `/healthz` endpoint is the supported
  liveness signal; probe it from outside the container. This is also more
  honest: Docker's healthcheck never restarted anything, so it only ever
  displayed a status.

- **The sample Compose file no longer sets `DIGITDNS_HEALTH_PORT`.** Nothing in
  the image reads it now.

This release also carries the v5.8.18 fixes: the quick-action menu now opens on
a scope that covers the host you clicked (so `*.host` no longer leaves the apex
looking direct), Add moves a domain instead of leaving a duplicate in Direct,
and Force on the resolver card waits for an in-flight benchmark instead of
returning "A benchmark is already in progress".

## Monitoring `/healthz`

A ready-made systemd timer, installed as root. It probes every 30 seconds and
restarts the container after three consecutive failures.

`/usr/local/bin/digitdns-healthcheck.sh`:

```sh
#!/bin/sh
set -u

URL="https://127.0.0.1:27586/healthz"
STATE="/run/digitdns-healthcheck.failures"

if curl -skf --max-time 5 "$URL" >/dev/null 2>&1; then
    rm -f "$STATE"
    exit 0
fi

n=$(cat "$STATE" 2>/dev/null || echo 0)
case "$n" in
    ''|*[!0-9]*) n=0 ;;
esac
n=$((n + 1))
echo "$n" > "$STATE"
logger -t digitdns-healthcheck "health probe failed ($n/3)"

if [ "$n" -ge 3 ]; then
    rm -f "$STATE"
    logger -t digitdns-healthcheck "restarting digitdns after 3 failed probes"
    /usr/bin/docker restart digitdns >/dev/null 2>&1 || true
fi
```

`/etc/systemd/system/digitdns-healthcheck.service`:

```ini
[Unit]
Description=DigitDNS panel health probe
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
Nice=10
ExecStart=/usr/local/bin/digitdns-healthcheck.sh
```

`/etc/systemd/system/digitdns-healthcheck.timer`:

```ini
[Unit]
Description=Probe the DigitDNS panel every 30 seconds

[Timer]
OnBootSec=2min
OnUnitActiveSec=30s
AccuracySec=5s
Unit=digitdns-healthcheck.service

[Install]
WantedBy=timers.target
```

Enable it:

```bash
chmod 755 /usr/local/bin/digitdns-healthcheck.sh
systemctl daemon-reload
systemctl enable --now digitdns-healthcheck.timer
```

Adjust the port in the script if `web_port` is not 27586. A plain cron entry or
an uptime service works too; the endpoint is unauthenticated and contentless.

If you keep your own exec healthcheck anyway, set a high `pids_limit` (for
example `1048576`) and a long interval, or the leak will eventually wedge the
container on an affected kernel.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.19` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.19/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.19
docker pull ghcr.io/digitrob/digitdns-agent:5.8.19
docker pull robmeister/digitdns:5.8.19
docker pull robmeister/digitdns-agent:5.8.19
```
