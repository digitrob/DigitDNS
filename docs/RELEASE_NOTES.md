# DigitDNS v4.0.0

DigitDNS can now exit in another country without moving the hub. A small relay
agent runs on a VPS where the service is reachable, the hub keeps the panel,
subscriber database, certificates and rules, and only the encrypted stream
travels through the agent.

That is the main idea behind 4.0.0. It also comes with a proper agent page on
both clients, per-domain and per-service relay choices, a real connection test,
and agent binaries and a container image built alongside the hub.

## Highlights

- **Geo relay agents.** Run `digitdns-agent` on a public IPv4 in another
  country. It has no panel, database or DNS server; it receives a host and
  client allowlist from the hub, relays TLS byte-for-byte, and reports traffic.
- **Relay control where the names are.** Every proxied domain can use **This
  Server**, inherit its service default, or name an agent. Services can carry a
  default relay, and a bulk **Egress** action moves a whole selection at once.
- **A real agent page.** Web and Android both have **Settings → Relay Agents**
  with add/edit/delete, enable/disable, key rotation and copy, country flags,
  last heartbeat, traffic, assigned domains and service defaults. The card being
  edited is outlined in cyan on the web.
- **A real test, not a guess.** **Test connection** opens a TCP connection to the
  agent's advertised relay port and records the latency. The badge uses the same
  green / amber / red thresholds as the upstream resolver rows.
- **Android parity.** Relay Agents sits beside Servers, the cards copy IP and
  key and rotate the key, and domain rows use an icon-only service mark and
  relay flag so the gear, status pill and switch stay exactly where they were.
- **Same delivery as the hub.** Release assets include
  `digitdns-agent-linux-amd64` and `digitdns-agent-linux-arm64`; the multi-arch
  `ghcr.io/digitrob/digitdns-agent` image is signed with cosign.

## How it behaves

A domain exits through, in order:

1. its own relay choice (**This Server**, an agent, or **Inherit**);
2. the service's default relay;
3. This Server.

The agent is IPv4-only and needs no certificate or domain of its own. The hub
still decides whether a client is allowed; the agent re-checks the client
address and destination against the catalogue it received. A hub outage does not
stop a working agent — it keeps the last catalogue and reconnects.

## Install the hub

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v4.0.0/scripts/install.sh | sudo bash
```

Docker: `docker pull ghcr.io/digitrob/digitdns:latest` (or `4.0.0`).

## Install an agent

Create it under **Settings → Relay Agents**, copy the key, then on the agent VPS:

```bash
mkdir -p /opt/digitdns-agent
docker run -d --name digitdns-agent --network host --restart unless-stopped \
  -v /opt/digitdns-agent:/data \
  ghcr.io/digitrob/digitdns-agent:latest \
  -hub 'https://panel.example.com:8080/ADMIN_PATH' \
  -key 'AGENT_ID:SECRET' \
  -listen :80,:443 \
  -state /data/agent-state.json
```

Open TCP 80/443 on the agent and allow outbound HTTPS to the hub. No inbound
admin port is opened. Full manual: [docs/AGENT.md](AGENT.md).

## Upgrade notes

- The database is migrated automatically; existing domain, service and client
  data is untouched.
- An install with no agents behaves exactly as 3.8.0 did. **This Server** remains
  the default.
- Hub binary: replace `digitdns` and restart.
- Hub Docker: pull the new `latest` / `4.0.0` image.
- Agent: replace the binary or pull the new image, keeping
  `/data/agent-state.json`.
- Agent keys created before encrypted copy storage existed must be rotated once
  before the card can copy them again.
- Android is version 4.0.0 (versionCode 400) and is attached to this release.

Back up `data.db` and `master.key` before upgrading.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `checksums.txt`, `checksums.sig`
- Android APKs for `arm64-v8a`, `armeabi-v7a`, `x86_64` and `universal`

Full list of changes: [docs/CHANGELOG.md](CHANGELOG.md).
