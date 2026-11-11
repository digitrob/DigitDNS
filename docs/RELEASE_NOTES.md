# DigitDNS v5.5.4 — setup checklist, change previews and working API docs

A feature and polish release on top of v5.5.3. No resolver, API or database
schema migration is required. Existing settings, clients, blocklists and
certificates are preserved.

## Panel

- **First-run checklist.** The dashboard now shows six live steps: replace the
  default admin password, enable 2FA, set the panel domain/certificate, confirm
  a DNS listener, add an upstream and create the first client. The state is read
  from the running install, so a restore or manual change moves it automatically.
- **Change preview before saving.** The explicit Save buttons for routing rules
  and ad-blocking settings show a redacted before/after diff and ask for
  confirmation. Background saves from per-domain toggles skip the modal.
- **Server Update card stays expanded on Docker.** The GitHub check no longer
  inherits the browser request's cancellation, results are cached for a minute,
  and a failed check still returns `container`, `update_mode`,
  `container_swap_active` and `container_image_version`.
- **2FA enrollment can be cancelled.** An abandoned enrollment no longer leaves
  the settings card expanded after a reload.
- **Ad Blocking Rule Tester** sits directly below the settings card, at half
  width on desktop and stacked on mobile.

## API

- **The v3 docs button works on a loopback-only `api_bind`.** The docs page
  accepts the dashboard's HttpOnly document cookie, inlines the spec, and loads
  the Swagger bootstrap from an external file the CSP permits.
- **The v3 spec now covers every router route**, with descriptions, request
  bodies, examples and a route/spec drift test.
- The v2 docs page uses the same inlined, CSP-safe bootstrap.

## Branding

- Startup splash, dashboard footer, About card, subscriber portal footer and
  systemd unit now use the complete SmartDNS, SNI-proxy and ad-blocking wording.
- Visible documentation no longer advertises old v2.x version headings.

## Android

- App version **5.5.4 (545)**. No functional Android changes in this release;
  the APK is rebuilt so its version matches the server.

## Upgrade

Docker:

```bash
docker compose pull
docker compose up -d
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.5.4
docker pull ghcr.io/digitrob/digitdns-agent:5.5.4
docker pull robmeister/digitdns:5.5.4
docker pull robmeister/digitdns-agent:5.5.4
```

systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.5.4/scripts/install.sh | sudo bash
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.5.4-linux-arm64-image.tar.gz` and `.sha256`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.5.4` tag.
