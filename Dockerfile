# syntax=docker/dockerfile:1

# ------------------------------------------------------------------------------
# Build Stage
# ------------------------------------------------------------------------------
# The tag floats inside the 1.26 series on purpose: go.mod asks for 1.26.4, and
# when the image happens to carry an older patch the default GOTOOLCHAIN=auto
# fetches the one the module names rather than refusing to build it.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /src

# ca-certificates is for the module fetch; git is what the module proxy needs for
# any dependency it cannot serve as a zip.
RUN apk add --no-cache git ca-certificates

# Copied first and on their own so the module download is cached against these two
# files rather than against every source edit. The cache mount persists the module
# download between builds, so a source-only change re-uses it instead of refetching.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO_ENABLED=0 is what makes the result runnable on a distroless or scratch base
# and on a VPS with no matching libc. -trimpath and -buildvcs=false keep the build
# path and the VCS stamp out of the binary, so two people building the same commit
# get the same bytes.
#
# TARGETOS/TARGETARCH are what buildx fills in per --platform: under a multi-arch
# build they let the Go compile cross natively (amd64 runner building arm64) with
# CGO off, instead of emulating the whole compiler through QEMU. The defaults keep
# a plain `docker build` working on the host's own architecture. The build cache
# mount keeps compiled packages between runs, which is where most of the time goes.
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/digitdns ./cmd/digitdns

# ------------------------------------------------------------------------------
# Final Runtime Stage
# ------------------------------------------------------------------------------
FROM alpine:3.20

WORKDIR /app

# The in-app updater swaps a binary for systemd installs. That is not how a
# container is deployed: the image is the artefact, and the next recreate would
# discard the swap. Mark the runtime so the updater refuses apply and tells the
# operator to pull/recreate instead.
ENV DIGITDNS_CONTAINER=1

# ca-certificates: the ACME client and the outbound public-IP probe both need a
# trust store. tzdata: expiry timestamps and quota cycles are rendered in the
# operator's zone, and without this every container is UTC whatever TZ says.
#
# The data/certs directories are created in the same layer, as well as declared as
# volumes below, so a run with no bind mount still has somewhere to put the key:
# the master key is written before anything else, and a missing directory there is
# a fatal start rather than a warning. One RUN, one layer, one apk index fetch.
RUN apk add --no-cache ca-certificates tzdata \
 && mkdir -p /app/data /app/certs

COPY --from=builder /out/digitdns /app/digitdns

# DNS (53), SNI proxy (80/443), DoT (853), DoQ (853/udp), dashboard (8080),
# HTTPS/DoH (8443), DoH3 (8443/udp) and DNSCrypt (5443/udp+tcp).
# Decorative under `network_mode: host`, which is what docker-compose.yml uses.
EXPOSE 53/udp 53/tcp 80/tcp 443/tcp 853/tcp 853/udp 8080/tcp 8443/tcp 8443/udp 5443/tcp 5443/udp

VOLUME ["/app/data", "/app/certs"]

# Reports the daemon as unhealthy while the panel listener is not accepting
# connections. The port is a variable because web_port is an operator setting: a
# check that hardcodes 8080 reports a container as sick for the sole reason that
# its owner moved the dashboard.
#
# This is a TCP connect, not an HTTP request, on purpose. A full check needs TLS
# whenever panel_https is on, and the TLS client used by the old probe (BusyBox
# wget) leaks one task slot in the container's cgroup on some kernels. Observed
# on Ubuntu 26.04 / 7.0.0-43: pids.current grew by exactly one per HTTPS probe
# while cgroup.threads stayed constant. At the default 30s interval the systemd
# DefaultTasksMax of 1066 was reached in about nine hours; from then on every
# exec, including the healthcheck itself, failed with "procReady not received"
# and a perfectly healthy resolver was reported unhealthy forever. nc -z is a
# single-threaded TCP connect: no TLS, no leak, and it works for both HTTP and
# HTTPS panels. /healthz remains the endpoint for external HTTP monitoring.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD nc -z -w 2 127.0.0.1 "${DIGITDNS_HEALTH_PORT:-8080}" || exit 1

# -daemon is not optional here, for the same reason scripts/digitdns.service passes
# it: a container must never be left guessing whether it is a server. `docker run -d`
# and `docker compose up -d` both hand the process /dev/null on stdin, and /dev/null
# is a character device, so the old "is stdin a terminal?" heuristic answered yes,
# started the interactive menu, read EOF and exited. The container came up, printed
# the menu, and died inside a second. The heuristic is fixed too — but a deployment
# entry point has no business depending on a heuristic at all.
#
# -db and -key are passed explicitly, and this is not a style choice. Without them
# the daemon resolves both relative to the working directory — /opt/digitdns does
# not exist in this image, so it falls back to ./data.db and ./master.key — which
# puts them in /app, inside the container's writable layer and outside the volume
# declared right above. `docker compose up --build`, or anything else that
# recreates the container, would then start against an empty database *and* a
# freshly generated master key: every subscriber, the admin password and the API
# key gone, and the previous data.db no longer decryptable even if a copy of it
# survived. All three persistent artefacts belong under /app/data.
ENTRYPOINT ["/app/digitdns", "-daemon", \
    "-config", "/app/data/config.json", \
    "-db", "/app/data/data.db", \
    "-key", "/app/data/master.key"]
