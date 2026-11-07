# DigitDNS v5.2.2 — optional IPv6 for the hub and relay agents

**A feature patch on the 5.2 line.** IPv6 for proxied names is now optional and
controlled end to end: a hub-wide master switch plus a per-agent opt-in. The
same release removes the transient “session expired” flash at sign-in and makes
the Android Relay Agents and Traffic History cards fit a phone screen.

There are no breaking API changes and no manual database migration. IPv6
defaults off for existing installs and agents, so proxied names keep their
current IPv4-only behaviour until you enable it.

## Optional IPv6

- New server-wide `proxy_ipv6_enabled` master switch and `public_ipv6` address.
  Proxied AAAA is published only when both are set; direct names keep their
  normal AAAA.
- New per-agent `ipv6_enabled` opt-in and `public_ipv6` address. Effective
  policy is the per-agent opt-in **and** the global switch.
- The hub delivers the effective policy in the agent catalogue and heartbeat
  reply. An agent applies a change on its next heartbeat, within 30 seconds.
- Turning IPv6 off makes an agent stop IPv6 detection and reporting and rebind
  IPv4-only. It returns to dual-stack when both switches are on again.
- Proxied AAAA is withheld while IPv6 is off. Proxied `HTTPS`/`SVCB` hints stay
  sunk even if proxy leak protection is disabled.
- New settings routes:
  - `GET/POST /api/settings` with `public_ipv6` and `proxy_ipv6_enabled`
  - `PUT /api/agents/{id}` with `public_ipv6` and `ipv6_enabled`
  - The v3 API exposes the same fields for Android and API-key clients.

## Web panel

- New **Settings → Proxy IPv6** card with the live switch, public IPv6 field,
  leak warning and saved state.
- Relay-agent form gains **Public IPv6** and **Use IPv6 for this agent**; the
  agent cards show the reported IPv6 address and effective state.
- Sign-in no longer flashes “session expired”. A 401 re-raises the login gate
  silently, the history panel stops writing the auth error into its card, and
  the login overlay stays up until the document reloads.

## Android companion

- New **Settings → IPv6** card with the live switch, public address, warning,
  save action and a confirmation before disabling.
- Relay-agent editor gains **Public IPv6** and **Use IPv6 for this agent**.
- Relay Agents cards are compact enough for two agents without scrolling; the
  list scrolls from the third agent.
- The Traffic History card fits one screen; the explanatory paragraphs above
  Save were removed and the fields were tightened.
- Added API, ViewModel and per-agent switch tests.

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
docker pull ghcr.io/digitrob/digitdns:5.2.2
docker pull robmeister/digitdns:5.2.2
```

Enabling IPv6 requires a global IPv6 address. Agents receive the policy on
their next heartbeat; no agent restart is required.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.2-linux-arm64-image.tar.gz`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

The Android APKs are attached to this release from the `android-v5.2.2` tag.
