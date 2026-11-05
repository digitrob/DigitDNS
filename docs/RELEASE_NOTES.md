# DigitDNS v5.0.4 — Real arm64 hub and agent images

**Important patch.** The arm64 image labels in **5.0.3 and earlier** contained
x86-64 binaries. On an ARM host that produced:

```text
exec /usr/local/bin/digitdns-agent: exec format error
```

5.0.4 rebuilds both `digitdns` and `digitdns-agent` correctly for
`linux/amd64` and `linux/arm64`.

## Root cause

Both Dockerfiles used:

```dockerfile
ARG TARGETOS=linux
ARG TARGETARCH=amd64
```

The explicit `amd64` default overrode BuildKit's per-platform `TARGETARCH`, so
even the arm64 manifest layer was compiled for amd64. The final Alpine stage was
arm64, but the copied Go binary was not.

### Fix

- Builder stage now runs on `$BUILDPLATFORM`.
- `TARGETOS`/`TARGETARCH` are redeclared without defaults.
- The Go build uses the BuildKit values, with host fallbacks for plain builds.

## CI now executes arm64 images

The Docker workflow no longer trusts manifest labels alone. After publishing it
runs both images under QEMU:

```bash
docker run --rm --platform linux/arm64 --entrypoint /app/digitdns <image> -version
docker run --rm --platform linux/arm64 --entrypoint /usr/local/bin/digitdns-agent <image> -version
```

A wrong-architecture binary fails the image build instead of reaching a release.

## Upgrading an ARM agent

Remove the old cached image and container, then pull the fixed tag:

```bash
docker rm -f digitdns-agent 2>/dev/null || true
docker image rm ghcr.io/digitrob/digitdns-agent:latest 2>/dev/null || true

docker pull --platform linux/arm64 ghcr.io/digitrob/digitdns-agent:5.0.4
docker run -d --name digitdns-agent \
  --platform linux/arm64 \
  --pull always \
  --network host --restart unless-stopped \
  -v /opt/digitdns-agent:/data \
  ghcr.io/digitrob/digitdns-agent:5.0.4 \
  -hub 'https://<your-panel>/<admin-path>' \
  -key '<agent-key>' \
  -listen :80,:443 \
  -state /data/agent-state.json
```

Verify:

```bash
docker exec digitdns-agent /usr/local/bin/digitdns-agent -version
docker image inspect ghcr.io/digitrob/digitdns-agent:5.0.4 \
  --format '{{.Os}}/{{.Architecture}}'
```

The hub image is fixed the same way.

## ⬆️ General upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:5.0.4
docker pull ghcr.io/digitrob/digitdns-agent:5.0.4
```

Android APKs are attached as `android-v5.0.4` (arm64-v8a, armeabi-v7a, x86_64
and universal).

## 📦 Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Full changes: [docs/CHANGELOG.md](https://github.com/digitrob/DigitDNS/blob/v5.0.4/docs/CHANGELOG.md).
