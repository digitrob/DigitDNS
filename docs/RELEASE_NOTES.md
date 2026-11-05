# DigitDNS v5.0.3 — Multi-arch agent setup, made copyable

A small patch over **5.0.2**. No database migration, no configuration changes.

## 🛰️ Relay-agent image

- Confirmed the published `ghcr.io/digitrob/digitdns-agent` image is
  multi-arch: **linux/amd64** and **linux/arm64**.
- Added a Docker CI gate that inspects the pushed agent image index and fails
  unless both platforms are present.
- No Dockerfile change was needed; the image already cross-compiles via
  `TARGETOS/TARGETARCH`.

## 📋 Agent creation box

- Added an **Auto / amd64 / arm64** selector for the generated `docker run`
  command.
  - **Auto** lets Docker choose the host architecture.
  - **amd64** adds `--platform linux/amd64`.
  - **arm64** adds `--platform linux/arm64` for Raspberry Pi and ARM VPS hosts.
- Generated commands now include **`--pull always`**, so a cached amd64
  `latest` cannot be reused on an ARM host.
- Added a **copy icon** beside the Docker command; it copies exactly the
  generated command including the selected platform.
- The agent-key copy button now uses the shared clipboard helper with its
  fallback path.
- Added a hint: Auto follows the host; choose arm64 on ARM.

## 🧪 Tests

- Web UI regression test covers the copy-command button, platform selector and
  the `--pull always` / `--platform linux/` command contract.
- Docker workflow YAML validation; web Node tests and Go web tests pass.

## ⬆️ Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:5.0.3
docker pull ghcr.io/digitrob/digitdns-agent:5.0.3
```

For an ARM agent host:

```bash
docker pull --platform linux/arm64 ghcr.io/digitrob/digitdns-agent:5.0.3
```

Android APKs are attached as `android-v5.0.3` (arm64-v8a, armeabi-v7a, x86_64
and universal).

## 📦 Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Full changes: [docs/CHANGELOG.md](https://github.com/digitrob/DigitDNS/blob/v5.0.3/docs/CHANGELOG.md).
