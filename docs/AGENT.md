# Geo relay agents

A relay agent is a small, optional process that runs on a public IPv4 address in
another country. The hub keeps doing the DNS work, the client checks, the
subscriber accounting and the panel; the agent only carries the encrypted
stream to the real destination.

The point is geography. A service may be reachable from Germany but not from the
country where the hub sits. Put an agent in Germany, assign that service to it,
and the viewer's traffic leaves through Germany while the hub, the database,
the certificates and every credential stay where they are.

An agent is **not** a second DigitDNS:

- no panel and no database;
- no DNS server and no certificate;
- no inbound administration port;
- no subscriber records — only a host allowlist and a client allowlist
  computed by the hub.

That is a deliberate security boundary. A box in a foreign datacentre should be
cheap to replace and should never hold the keys to your subscriber database.

---

## How the pieces fit

```
client ──DNS──▶ hub:53            resolves the domain
client ──TLS──▶ hub:443           hub says "this domain leaves through agent X"
                    │
                    ▼
             agent X in DE:443    forwards the encrypted bytes to the origin
                    │
                    ▼
                 origin.example
```

The client's TLS session is passed through untouched. The agent does not
decrypt, re-sign or terminate it, so it needs no certificate for the destination
and no agent domain. The hub still decides whether a client is allowed; the
agent re-checks the client address and the destination host against the
allowlist it received.

### What the agent syncs

- **Catalogue:** every active proxied host assigned to it directly or through a
  service default, plus every client address the hub would admit.
- **Heartbeat:** version, advertised relay port, bytes in/out, and the public
  address the request arrived from. The address is learned from the connection,
  not trusted from the payload.
- A hub outage does not stop the relay. The agent keeps the last catalogue and
  keeps serving; reconnects are logged and retried.

---

## Requirements

| Item | Requirement |
| :--- | :--- |
| Address | A public **IPv4** address. Agents are IPv4-only. |
| Inbound | TCP **80** and **443** for relayed traffic. |
| Outbound | HTTPS to the hub panel origin (the same URL you use in a browser). |
| OS | Linux amd64 or arm64 for the binary; any Docker host for the image. |
| Certificate | None on the agent. |
| Domain | None on the agent. |
| Resources | 256–512 MB RAM is enough for a small agent; the relay is stream I/O. |

Do not put the agent behind a TLS-terminating proxy or a CDN. It must receive
the client connection and the destination's TLS bytes unchanged.

---

## 1. Create the agent on the hub

1. Sign in to the hub panel.
2. Open **Settings → Relay Agents**.
3. Press **Add agent**, give it a name you will recognise later (for example
   `Frankfurt 2`), and choose the country it exits as.
4. Press **Create agent** and copy the key. It is shown once here, but the hub
   also stores an encrypted copy so the card's copy button can return it later.

The card starts **offline**. It becomes **online** after the first heartbeat,
which is sent every 30 seconds.

The country on the card is a label and the flag shown in relay pickers. It does
not change routing by itself.

---

## 2. Install the agent

### Docker (recommended)

```bash
docker pull ghcr.io/digitrob/digitdns-agent:latest
# Docker Hub mirror:
# docker pull robmeister/digitdns-agent:latest

mkdir -p /opt/digitdns-agent

docker run -d --name digitdns-agent --network host --restart unless-stopped \
  -v /opt/digitdns-agent:/data \
  ghcr.io/digitrob/digitdns-agent:latest \
  -hub 'https://panel.example.com:8080/ADMIN_PATH' \
  -key 'AGENT_ID:SECRET' \
  -listen :80,:443 \
  -state /data/agent-state.json
```

- `--network host` is required. The agent must see the client's real address and
  bind 80/443 without Docker port remapping.
- `-hub` is the panel origin **including the hidden admin path**, with no
  trailing `/agent`. The agent appends `/agent/heartbeat` and
  `/agent/catalogue` itself.
- `-key` is the `id:secret` value from the card. After the first run the key is
  cached in `/data/agent-state.json`; later restarts can omit `-key`.
- `-listen :80,:443` is the default. If you change it, make sure the agent's
  advertised port is reachable from the hub — the Test connection button dials
  the reported port.
- `-state /data/agent-state.json` keeps the key and the last catalogue across
  container recreation. Back up `/opt/digitdns-agent`.
- IPv6 is controlled by the hub. Enabling **Use IPv6 for this agent** in the
  panel (and the global **Proxy IPv6** switch) is what makes the agent publish
  AAAA and listen on IPv6. `-public-ipv6` supplies the address when automatic
  detection cannot; the agent applies a policy change on its next heartbeat
  (within 30 seconds), with no restart. When the policy is off, the agent
  rebinds IPv4-only and reports no IPv6.

The image is multi-arch (`linux/amd64`, `linux/arm64`) and cosign-signed with
the same keyless identity as the hub image. The agent creation box always tracks the
`latest` image and adds `--pull always` / `pull_policy: always`; choose `arm64`
there for a Raspberry Pi or ARM VPS.

### Registry connectivity

If `docker pull`/`docker run` fails with `EOF` while contacting `ghcr.io`, the
image is usually fine and the host cannot complete the registry request:

```bash
uname -m
getent ahosts ghcr.io
curl -4 -v --max-time 15 https://ghcr.io/v2/
curl -6 -v --max-time 15 https://ghcr.io/v2/
sudo systemctl restart docker
```

Check broken IPv6 first if `-4` works and `-6` fails, then DNS/MTU/proxy
configuration. Every arm64 release also publishes a ready-to-load image, so GHCR is not needed
on the agent host:

```bash
curl -fL -o /tmp/agent-image.tar.gz \
  https://github.com/digitrob/DigitDNS/releases/download/v5.2.12/digitdns-agent-5.2.12-linux-arm64-image.tar.gz
gunzip -c /tmp/agent-image.tar.gz | docker load
docker tag ghcr.io/digitrob/digitdns-agent:5.2.12 ghcr.io/digitrob/digitdns-agent:latest
```

As an offline fallback from another machine, save the image and load it on the
agent host:

```bash
# on a connected machine
docker pull --platform linux/arm64 ghcr.io/digitrob/digitdns-agent:5.2.12
docker save ghcr.io/digitrob/digitdns-agent:5.2.12 | gzip > agent-arm64.tar.gz
scp agent-arm64.tar.gz admin@agent-host:/tmp/

# on the agent host
gunzip -c /tmp/agent-arm64.tar.gz | docker load
docker run -d --name digitdns-agent --pull never --network host \
  --restart unless-stopped -v /opt/digitdns-agent:/data \
  ghcr.io/digitrob/digitdns-agent:5.2.12 \
  -hub 'https://panel.example.com:8080/ADMIN_PATH' \
  -key 'AGENT_ID:SECRET' -listen :80,:443 \
  -state /data/agent-state.json
```

The release also ships `digitdns-agent-linux-arm64` if Docker cannot be used at
all.

Verify it:

```bash
cosign verify ghcr.io/digitrob/digitdns-agent:latest \
  --certificate-identity-regexp 'https://github.com/digitrob/DigitDNS/.github/workflows/docker.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

### Portainer / Docker Compose

The agent creation box also generates a stack for Portainer. Use **Stacks → Add
stack → Web editor**, paste the generated YAML and deploy it. The generated
snippet uses the same pinned image and architecture selector as the Docker
command:

```yaml
services:
  digitdns-agent:
    image: ghcr.io/digitrob/digitdns-agent:latest
    pull_policy: always
    container_name: digitdns-agent
    platform: linux/arm64
    network_mode: host
    restart: unless-stopped
    volumes:
      - /opt/digitdns-agent:/data
    command:
      - "-hub"
      - "https://panel.example.com:8080/ADMIN_PATH"
      - "-key"
      - "AGENT_ID:SECRET"
      - "-listen"
      - ":80,:443"
      - "-state"
      - "/data/agent-state.json"
```

Choose `arm64` for a Raspberry Pi / ARM VPS; omit `platform` for Auto. The key
is quoted because it contains a colon. `/opt/digitdns-agent` remains the state
volume.

### Binary + systemd

Download the matching binary from the release page:

```bash
# amd64
curl -fL -o /usr/local/bin/digitdns-agent \
  https://github.com/digitrob/DigitDNS/releases/download/v5.2.12/digitdns-agent-linux-amd64
chmod +x /usr/local/bin/digitdns-agent

# arm64: use digitdns-agent-linux-arm64 instead
```

Create the state directory and a unit:

```ini
# /etc/systemd/system/digitdns-agent.service
[Unit]
Description=DigitDNS relay agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/digitdns-agent \
  -hub https://panel.example.com:8080/ADMIN_PATH \
  -key AGENT_ID:SECRET \
  -listen :80,:443 \
  -state /var/lib/digitdns-agent/state.json
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

```bash
sudo mkdir -p /var/lib/digitdns-agent
sudo systemctl daemon-reload
sudo systemctl enable --now digitdns-agent
sudo journalctl -u digitdns-agent -f
```

Running as root is only for binding ports 80/443. The agent exposes no admin
surface.

### First run without `-key` (enrolment token)

If you created the agent before the key was shown, or your deployment process
hands out a one-time token instead, run once with `-token`:

```bash
digitdns-agent -hub 'https://panel.example.com:8080/ADMIN_PATH' \
  -token 'ONE_TIME_TOKEN' -listen :80,:443 -state /var/lib/digitdns-agent/state.json
```

The token is spent after the first successful enrolment. The returned key is
saved in the state file.

---

## 3. Confirm it works

On the hub card:

- the status changes to **online** within roughly 30 seconds;
- **Last seen** is under two minutes old;
- **Test connection** opens a real TCP connection to the advertised relay port
  and records the latency. The badge is green under 40 ms, amber over 40 ms and
  red over 80 ms. A failed connect reports the address it tried.

In the agent log:

```
[agent] v5.2.12-stable · country DE · IPv6 on · public IPv6 2001:db8::2 · listening :80,:443 · dialling http :80 / https :443
[agent] 2 hosts allowed · 3 client entries · private guard blocked
```

`0 hosts allowed` or `0 client entries` means the catalogue is empty. Assign a
domain or a service default to the agent; the change normally arrives in under a
second through the catalogue long-poll, with a five-minute full sync as the
safety net.

---

## 4. Assign traffic

Relay resolution has three levels, highest first:

1. **Domain override** — the relay control on the domain row.
   - **This Server** writes `__local__` and forces the local exit.
   - **Inherit service** removes the entry so the service default applies.
   - **A named agent** writes that agent's id.
2. **Service default** — the **Default relay** picker in the service editor.
   This is the default for every domain in that service that has no override.
3. **Local fallback** — with neither, the hub relays locally.

The web and Android both offer a bulk **Egress** action for a selection. It
writes one domain override per selected domain and does not change their
service labels or routing status.

Assignments are stored as `domain_agents` and `service_agents` in the rules
store. They are part of the rules API and survive a partial save.

---

## 5. Operate and update

### Card controls

| Control | Effect |
| :--- | :--- |
| Edit | Change name, country, the reported public IPv4/IPv6, or the per-agent IPv6 opt-in. |
| Pause / play | Disable or re-enable the agent. Disabled agents are not selected for DNS answers. |
| Rotate key | Issue a new key. The old key stops working immediately. |
| Copy key | Return the encrypted clear-key copy. A legacy key answers 409 and must be rotated once. |
| Delete | Remove the agent and prune every domain/service assignment that pointed at it. |
| Test connection | TCP connect to the relay port, store `ping_ms` / `ping_at`, update the badge. |

### Update the agent

**Docker**

```bash
docker pull ghcr.io/digitrob/digitdns-agent:latest
docker rm -f digitdns-agent
# run the same docker run command again; /opt/digitdns-agent keeps the state
```

**Binary**

Replace `/usr/local/bin/digitdns-agent` and restart the unit.

### Rotate a key

1. Press **Rotate key** on the card and copy the new key.
2. Update the agent's `-key` (or state file) and restart it.
3. The old key is invalid as soon as rotation returns; the card will go offline
   until the agent presents the new one.

### Back up

The only state is the state file:

```bash
sudo tar -czf /root/digitdns-agent-$(date +%F).tar.gz \
  -C /var/lib/digitdns-agent state.json
```

Losing it costs a new key, not the agent's identity in the panel; rotating and
reinstalling is safe.

### Remove

Delete the agent in the panel first (so assignments fall back to This Server),
then stop and remove the container or unit and its state directory.

---

## Flags

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `-hub` | — | Hub base URL including the hidden admin path. |
| `-key` | — | Agent key in `id:secret` form. Cached in the state file. |
| `-token` | — | One-time enrolment token, if your flow uses one. |
| `-listen` | `:80,:443` | Comma-separated listen addresses. |
| `-port` | `443` | TLS port dialled on the far side (origin). |
| `-http-port` | `80` | Plain HTTP port dialled on the far side. |
| `-country` | — | Label for the agent's exit country. |
| `-public-ipv6` | auto-detect | Globally routable IPv6 this agent publishes for proxied AAAA. |
| `-state` | `agent-state.json` | Key and catalogue cache. |
| `-hosts` | — | Local host allowlist override, for testing. |
| `-clients` | — | Local client allowlist override, for testing. |
| `-allow-private` | off | Relay to private addresses. Testing only. |
| `-config` | — | JSON config file with the same fields. |
| `-write-config` | — | Write the effective config and exit. |
| `-version` | — | Print the version and exit. |

`-hosts`, `-clients` and `-allow-private` are for lab work. Leave them unset in
production; the hub's catalogue is the authority.

---

## Troubleshooting

| Symptom | What to check |
| :--- | :--- |
| Card stays offline | Agent can reach `-hub` over HTTPS; the admin path is included; the key matches; the hub's firewall allows it. |
| `401` in the agent log | Key is wrong, rotated or disabled. Copy the current key and restart. |
| Empty host allowlist | No active proxied domain or service default points at this agent. Assign one; it normally arrives in under a second. |
| Empty client allowlist | Hub is in whitelist mode with no registered clients; add clients or use the intended access mode. |
| Test connection fails | Advertised port is unreachable. Check TCP 80/443, host firewall and that `-listen` is not behind NAT. |
| Relay works but card is stale | Heartbeat path only; check outbound HTTPS and the hub clock. |
| `409` when copying the key | Legacy key without encrypted copy storage. Rotate once, then copy. |
| Agent refuses everything | Read the startup line: host/client counts and the private guard. |
| Certificate error to origin | The agent passes TLS through; a certificate error is between the client and the origin, not the agent. |

Logs:

```bash
docker logs -f digitdns-agent
sudo journalctl -u digitdns-agent -f
```

---

## Security notes

- Treat the agent key as a credential. It grants catalogue access and relay use,
  not panel access, but a leaked key lets someone impersonate the agent.
- Prefer passing the key once and letting the state file carry it afterwards;
  a key on a process command line is visible to local users in `ps`.
- The hub never puts a clear key in the agent list response. Copied keys come
  from an authenticated endpoint and are stored encrypted at rest.
- The agent opens no admin port. Inbound 80/443 is the relay only.
- Keep the host allowlist tight. The hub computes it from the domains assigned
  to the agent, so an unassigned agent refuses everything by design.
- The private-address guard is on by default. Do not disable it on a public box.
- Give the agent a current CA trust store so it can reach the hub over HTTPS.

---

## API

The control plane is also available over v3:

| Method | Route | Purpose |
| :--- | :--- | :--- |
| `GET` | `/api/v3/agents` | List agents. Never includes a clear key. |
| `POST` | `/api/v3/agents` | Create; returns the agent and its key once. |
| `PUT` | `/api/v3/agents/{id}` | Edit name, country, public IPv4. |
| `DELETE` | `/api/v3/agents/{id}` | Delete and prune assignments. |
| `POST` | `/api/v3/agents/{id}/enable` · `disable` | Enable or disable. |
| `POST` | `/api/v3/agents/{id}/rotate-key` | Rotate and return the new key. |
| `GET` | `/api/v3/agents/{id}/key` | Copy the encrypted clear key. |
| `POST` | `/api/v3/agents/{id}/test` | Real TCP test; stores `ping_ms` / `ping_at`. |

Agent-facing routes (`/agent/enrol`, `/agent/catalogue`, `/agent/heartbeat`) use
an agent credential — `X-Agent-Key` for catalogue and heartbeat, a one-time
token for the enrolment call — and are never reachable with a panel session.
