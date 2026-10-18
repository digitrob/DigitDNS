# ⚡ DigitDNS — Anti-Sanction SmartDNS & Gaming Gateway

[![Release](https://img.shields.io/github/v/release/digitrob/DigitDNS?display=tag&style=for-the-badge&label=Release)](https://github.com/digitrob/DigitDNS/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/digitrob/DigitDNS/ci.yml?branch=custom&style=for-the-badge&label=CI)](https://github.com/digitrob/DigitDNS/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/github/actions/workflow/status/digitrob/DigitDNS/docker.yml?branch=custom&style=for-the-badge&label=Docker)](https://github.com/digitrob/DigitDNS/actions/workflows/docker.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&label=Language)](https://go.dev)
[![Protocols](https://img.shields.io/badge/DNS-UDP%20%7C%20DoT%20%7C%20DoH%20%7C%20DoQ%20%7C%20DNSCrypt-10b981?style=for-the-badge)](#-4-encrypted-dns-that-reuses-its-connections)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge&logo=gnu)](LICENSE)

> **DigitDNS is a fork of [HyperDNS](https://github.com/IzumiRain/HyperDNS)** by HyperRAIN,
> maintained by [Digitrob](https://github.com/digitrob). The resolver engine, SNI relay,
> dashboard and REST API descend directly from upstream; this fork rebrands the product,
> ships its own signed auto-updates and container images, and adds encrypted upstream
> transports. Upstream's history, licence and RFC work are preserved — see
> [NOTICE](NOTICE) and the [License](#-license) section.
>
> This fork exists for **educational and self-hosting purposes** — to document, study and
> extend the upstream project. It is not a commercial product and comes with no warranty;
> run it only on servers and networks you are authorised to operate.

<p align="center">
  <img src="docs/digitdns-dashboard.png" alt="DigitDNS web administration dashboard" width="100%">
</p>

> [!IMPORTANT]
> ### v3.0.0 — The DigitDNS fork release
> The project is renamed **HyperDNS → DigitDNS** throughout: the install tree
> (`/opt/digitdns`), the systemd unit (`digitdns.service`), the console command
> (`digitdns`), every environment variable (`DIGITDNS_*`), the API-key prefix
> (`digitdns_live_`) and the release assets. **An existing HyperDNS install is migrated
> in place** — the installers archive the old tree to
> `/root/hyperdns-premigration-<ts>.tar.gz`, move it, and preserve `data.db`,
> `master.key`, `config.json` and certificates exactly; a stored `hdns_live_` API key is
> rotated to the new prefix once at daemon start. The fork also adds **encrypted
> upstream transports** (DoQ, DoH3 and DNSCrypt joins DoT/DoH in the Upstream Racers),
> a batch of dashboard UX improvements (Proxied Domains sort/bulk controls, Live Query
> Log row actions, bulk static-record import), an English-only UI, and a fix for DoT/DoH
> staying dark on installs whose certificate lives in the built-in ACME store. See
> [CHANGELOG.md](docs/CHANGELOG.md) for the full list and [NOTICE](NOTICE) for what
> deliberately stays byte-compatible with upstream.

<p align="center">
  <a href="#-quick-installation--deployment">🚀 Install</a> •
  <a href="#-option-2-docker-multi-arch-from-ghcr">🐳 Docker</a> •
  <a href="#-first-run-hardening">🔒 Harden</a> •
  <a href="#-key-features">🌟 Features</a> •
  <a href="#-web-administration-dashboard">🖥️ Dashboard</a> •
  <a href="docs/API.md">🔌 REST API</a> •
  <a href="#-technical-architecture--codebase-design">🏛️ Internals</a> •
  <a href="docs/SECURITY.md">🛡️ Security</a> •
  <a href="#-support--donate">☕ Support</a>
</p>

---

## 📚 Documentation

| Document | Covers |
| :--- | :--- |
| **[docs/TUTORIAL.md](docs/TUTORIAL.md)** | Step-by-step walkthrough: first boot, custom domain for the subscriber portal, adding clients, issuing registration tokens. **Start here.** |
| **[SECURITY.md](docs/SECURITY.md)** | Threat model, encryption specifics, hardening checklist, RFC compliance, vulnerability reporting. **Read before exposing the install.** |
| **[API.md](docs/API.md)** | REST API v1 (deprecated) and v2, with Python / Node.js / cURL samples. |
| **[CHANGELOG.md](docs/CHANGELOG.md)** | Release history and the fix behind each one. |
| **[docs/RELEASING.md](docs/RELEASING.md)** | Versioning policy, the stable/beta channels, and how the in-dashboard updater decides what to offer. |

## 📖 Overview

**DigitDNS** is an ultra-fast, single-binary SmartDNS server and transparent SNI Proxy engine with an embedded web dashboard, developer REST API, and interactive terminal interface (`digitdns`). It descends from [HyperDNS](https://github.com/IzumiRain/HyperDNS) by HyperRAIN.

It transforms any single cloud VPS (even a \$3/month 1-Core CPU / 1GB RAM instance) into a private **Anti-Sanction (403 Bypass)** and **Low-Latency Gaming Gateway** for **PC, PlayStation 4/5, Xbox Series X/S, Nintendo Switch, Android, iOS, and Routers** with:
- **0 Client Software Required:** Users only set their DNS IP address on their console or router.
- **0 VPN Overhead:** Game UDP traffic (voice, physics, tick rate) connects directly to official servers without double encryption or MTU packet fragmentation.
- **1-Click Self-Healing Registration:** Dynamic mobile & residential IPs update automatically via private one-click slugs (`/ip/{token}`).

---

## 🖥️ Server Requirements

DigitDNS is a single static Go binary. There is no database server, no runtime,
no container image underneath it — the sizing question is only how many
subscribers you serve and whether they relay downloads through you.

| Install class | vCPU | RAM | realistic for |
| :--- | :--- | :--- | :--- |
| **Floor** — runs, personal use | 1 | 1 GB | 1–3 devices, policies on, downloads direct. This is the smallest VPS the daemon starts on. |
| **Recommended** — small reseller | 2 | 2 GB | up to ~10 subscribers on gameplay-only plans, DoT + DoH + portal all enabled |
| **Comfortable** — many clients | 4 | 4 GB | 20+ subscribers, benchmark runs while serving |

Honest history behind these numbers: on **v1.2.0**, a 1-core / 1-GB box was
pegged at **100% CPU with just 3 clients** — that build also had a serious
memory leak that made the lag worse the longer it ran. v2.2.0 is a different
engine: the lookup and rule-match paths are allocation-free nanosecond reads
(measured, not estimated — see [Architecture](#%EF%B8%8F-technical-architecture--codebase-design)),
the cache is sharded and capped, a relayed connection no longer allocates its
own buffer, and the leak classes of that era are gone. A 1-core / 1-GB box is
now a *usable* personal install — but if you sell access, buy the 2-vCPU tier:
rate limiting, EDE generation and relay fan-out all cost CPU on the query path,
and the box that hosts your resolver should not be the bottleneck your ping
blames.

Also required: a **domain** pointed at the server (HTTPS is mandatory; the
daemon issues its own certificate), and these ports reachable:

| Port | Protocol | Purpose |
|------|----------|---------|
| **53** | UDP + TCP | DNS resolver — the core service |
| **443** | TCP | SNI relay for proxied game traffic + panel HTTPS |
| **80** | TCP | ACME HTTP-01 certificate issuance + SNI relay HTTP |
| **8443** | TCP | DoH (DNS-over-HTTPS) |
| **853** | TCP | DoT (DNS-over-TLS) |
| panel port | TCP | admin dashboard — a random high port, shown by the installer |

If another service already owns **53** (a local `masterdns`/dnsmasq is the
usual one), **443**, or **8443**, free it first or the daemon cannot bind that
listener at start. The installer lists these ports and warns about conflicts
before it installs anything.

### 🔢 Versioning & release policy

This project follows **[SemVer](https://semver.org)** — `MAJOR.MINOR.PATCH`:

- **MINOR** bump (`3.0.0` → `3.1.0`) for new features.
- **PATCH** bump (`3.0.0` → `3.0.1`) for backwards-compatible fixes.
- **MAJOR** bump for a breaking change.
- A pre-release tag (`v3.1.0-beta.1`) precedes the final `v3.1.0` of the same
  number — it is never *after* it.

**Two channels, one source of truth.** `version.json` in the repository is what
every build embeds and what the dashboard's update check reads. Releases
published from the default branch (`custom`) are the stable line; tags carrying
a `-beta` channel are published as **GitHub prereleases** and never take the
*Latest* badge, so the README's install one-liner and new users keep landing on
the stable line. Every release bundle is **ed25519-signed**, and the
in-dashboard updater installs only signed final releases — install a pre-release
by hand with `install.sh` if you want it.

**Where the update number comes from.** The dashboard does **not** read GitHub's
*Latest* release. It fetches `version.json` from the repository's default
branch, compares that number with the version this build embeds, then resolves
the release whose **tag matches that number exactly**. So the update feed moves
on exactly one event — the default branch's `version.json` bumping to a final
number, followed by that tag — and a `-beta` tag never satisfies it.

**Container images follow the same rules.** `ghcr.io/digitrob/digitdns:latest`
is always the newest stable release, `:edge` tracks the default branch
(development builds), and `:X.Y.Z` is an immutable release image. Every image
is multi-arch (amd64 + arm64) and cosign-signed — see [Docker](#-option-2-docker-multi-arch-from-ghcr).

Full detail — channels, the updater's decision order, and the tag-history note —
lives in **[docs/RELEASING.md](docs/RELEASING.md)**.

---

## 🚀 Quick Installation & Deployment

### Option 1: One-Line Linux Installer (Recommended)
Run as `root` on Ubuntu 20.04+, Debian 11+, or AlmaLinux/Rocky 8+:
```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v3.0.0/scripts/install.sh | sudo bash
```

> [!IMPORTANT]
> **The installer never upgrades in place by surprise.** A fresh install replaces
> whatever is there after archiving it to `/root/digitdns-preinstall-<date>.tar.gz`
> (its sha256 printed). An interactive run types `FRESH` to confirm the wipe; a
> piped `curl | bash` run refuses unless `DIGITDNS_FRESH=1` is set. **Coming from
> HyperDNS**: the installers detect a legacy `/opt/hyperdns` tree and migrate it in
> place — archive, move, rename the unit — preserving `data.db`, `master.key`,
> `config.json` and certificates, and skipping the wipe. Coming from **HyperDNS 1.x**,
> run `bash migrate-from-v1.sh` from the offline bundle first.
>
> **A panel domain is mandatory** — its A record must point at the server. The daemon issues the Let's Encrypt certificate **itself** over its own port-80 listener: no certbot, no acme.sh, nothing to stop the service for. Non-interactive: `DIGITDNS_DOMAIN=dns.example.com DIGITDNS_EMAIL=you@example.com bash install.sh`. If issuance fails the installer states the cause and exits rather than printing success. Renewal is daily and automatic.
>
> **Subscribers self-serve IP changes** through a secret: `/sub/<token>` is a read-only portal, and moving the binding is `POST /ip/<token>` with the account's registration secret. A leaked portal link alone cannot steal the binding.

### 🐳 Option 2: Docker (multi-arch, from GHCR)

Every release publishes a multi-arch image (**linux/amd64 + linux/arm64**) to
GitHub Container Registry, signed with keyless cosign:

```bash
docker pull ghcr.io/digitrob/digitdns:latest

mkdir -p data certs
docker run -d --name digitdns --network host \
  -v ./data:/app/data -v ./certs:/app/certs \
  -e TZ=UTC \
  ghcr.io/digitrob/digitdns:latest
```

Or with Compose — `docker-compose.yml`:

```yaml
services:
  digitdns:
    image: ghcr.io/digitrob/digitdns:latest   # or :edge for development builds
    container_name: digitdns
    restart: always
    network_mode: "host"                      # see the rationale below
    volumes:
      - ./data:/app/data
      - ./certs:/app/certs
    environment:
      - TZ=UTC
      - DIGITDNS_HEALTH_PORT=8080
    ulimits:
      nofile: { soft: 65535, hard: 65535 }
    logging:
      driver: "json-file"
      options: { max-size: "10m", max-file: "3" }
    stop_grace_period: 10s
```

```bash
mkdir -p data certs && docker compose up -d
```

Create `data/` and `certs/` before the first `up`. Everything that has to
survive a rebuild lives in `data/` — `data.db`, `master.key` and `config.json`.
`config.json` is optional: drop one in to seed the first run, or leave the
directory empty and let the daemon generate its own.

The container runs with `network_mode: host` on purpose. A NAT hop on every UDP datagram is latency added to the query that starts a game, and the daemon has to see the real client address — subscriber recognition, the per-source rate limit and the access list all key on it. Behind Docker's userland proxy every query would arrive from the bridge address, so one client's flood would rate-limit the whole subnet and every subscriber would look like the same subscriber.

**Image tags:**

| Tag | Meaning |
| :--- | :--- |
| `latest` | newest **stable** release — use this in production |
| `X.Y.Z` | an immutable release image (e.g. `3.0.0`) |
| `vX.Y.Z` | same, with the `v` prefix the git tag carries |
| `edge` | development build from the default branch — not for production |

**Verifying a signature** (keyless cosign; no key to manage — the signature is
bound to this repository and its Docker workflow by the Sigstore transparency log):

```bash
cosign verify ghcr.io/digitrob/digitdns:latest \
  --certificate-identity-regexp 'https://github.com/digitrob/DigitDNS/.github/workflows/docker.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Building the image locally still works and overrides the tag:
```bash
docker compose build   # uses the Dockerfile in this repository
```

### Option 3: Pre-Compiled Standalone Binary

Download from the [latest release](https://github.com/digitrob/DigitDNS/releases/latest)
(`digitdns-linux-amd64`, `digitdns-linux-arm64`, `digitdns-windows-amd64.exe`,
`digitdns-darwin-arm64`, `digitdns-darwin-amd64` — every asset is listed in the
release's signed `checksums.txt`):

```bash
# Linux amd64 example:
curl -fsSL -o digitdns https://github.com/digitrob/DigitDNS/releases/download/v3.0.0/digitdns-linux-amd64
chmod +x digitdns
sudo ./digitdns -server
```

### Option 4: Compile from Source
```bash
git clone https://github.com/digitrob/DigitDNS.git
cd "DigitDNS"
go build -ldflags="-s -w" -o bin/digitdns ./cmd/digitdns
./bin/digitdns -server
```

---

## 🌟 Key Features

### 🎮 1. Domain Rules You Control
This build ships **no built-in catalogue**. Routing is entirely the operator's, edited from the dashboard or the REST API and stored in the encrypted database:
- **Custom Proxied** — names relayed through the SNI proxy.
- **Custom Blocked** — names sinkholed to `0.0.0.0` (ads, telemetry, anything you want gone).
- **Custom Direct** — names forced to resolve normally, overriding a proxy rule for that name.
- **Named groups** — a set of domains sharing one action (`proxy` / `direct` / `block`), enabled or disabled as a unit, each with an optional uploaded icon (SVG ≤ 16 KB or PNG ≤ 64 KB, re-encoded through a server-side allowlist so scripts and external references cannot survive).
- **Bulk import** — add a whole list at once: paste it or upload a file (≤ 1 MiB), one name per line or comma-separated, `*.` wildcards and `#` comments understood. The import reports how many names were added, already present, and invalid.
- **Bulk static records** — load a `hostname address` table into **Custom Static Records** at once: paste or upload, IPv4 and IPv6 accepted; re-uploading a corrected table updates existing hostnames in place.
- **A small forced-direct plane stays built in.** A handful of names cannot work through a Layer-4 relay — Discord voice/video media, Vivox voice, Steam's connection manager, and Google's update CDNs — so they are always resolved directly, ahead of every rule, and cannot be lost to a wildcard. See [Bulk Downloads Stay Direct](#-6-bulk-downloads-stay-direct).

### 🛡️ 2. Transparent SNI Proxy Relay
- Layer-4 TCP relaying without SSL termination: the TLS session stays end-to-end between the client and the origin, and DigitDNS never holds the keys for it.
- Anti-DPI TLS ClientHello fragmentation, on by default (`enable_fragmentation`, with a configurable chunk size and inter-chunk delay).
- A `sync.Pool` of 32 KB buffers, so a relayed connection does not allocate its own — the payload is still copied through user space, and each direction carries an inactivity deadline that is refreshed per chunk so half-open connections are reaped instead of pinned.
- Up to 4096 concurrent relays; beyond that new connections are rejected rather than queued, which keeps a flood from exhausting memory.

### 👤 3. Subscriber Management & 1-Click IP Sync
- **Two addresses, two jobs.** `/sub/{token}` is the subscriber's **portal** — a
  read-only overview of their plan, quota and detected address, safe to hand out
  because a leaked link cannot move their binding. `/ip/{token}` is the
  **registration API**: `POST` with the account's 96-bit registration secret
  rebinds the address (send no `ip` field to bind the caller's own — the 1-click
  flow); a bare `GET` serves a bilingual explainer page for old bookmarks.
- **The registration secret travels out-of-band.** It is shown in the operator's
  client panel (and stored AES-256-GCM sealed beside the token), regenerable
  there. This is what makes a leaked portal link read-only material rather than
  a way to steal the subscription.
- **Registration gates:** suspended, expired and over-quota accounts are refused
  with a reason the portal can translate; an address already bound to a
  *different* subscription answers `409` **without disabling either account**,
  because shared CGNAT addresses are ordinary on mobile networks.
- **An optional dedicated portal port.** Set one in Settings → *Subscription
  Portal* and the daemon binds a second listener for the public subscriber
  routes only — the admin namespace, REST API, DoH and the SPA answer a bare 404
  there.
- **RFC 9562 v4 UUIDs:** stable unique identifiers for all clients (the standard that supersedes RFC 4122).
- **Plan Durations & Traffic Limits:** daily/monthly/yearly/lifetime expiry with
  live quota enforcement.
- **Telegram provisioning cards:** a 1-click formatted card carrying the
  portal link, the registration secret, the DNS address and the API URL.
- **Whitelist by default.** An unknown source is **refused** by DNS —
  with an RFC 8914 *Prohibited* Extended DNS Error for OPT-bearing queries, so
  the client learns *why* — instead of being served for free. Fresh installs
  ship whitelist-on; an existing install converges through a one-time
  idempotent migration on first boot. If you sell access, keep
  `access.allow_all: false` — under `allow_all` revocation does not work, because
  an unindexed address is still served as anonymous `Public`, which has no plan
  to expire and no allowance to exceed. See [First-Run Hardening](#-first-run-hardening).

### 🔐 4. Encrypted DNS That Reuses Its Connections
- **Plain UDP/TCP on 53, DNS-over-TLS on 853, DNS-over-HTTPS on its own port
  (8443 default, configurable)** — one daemon, one process. The DoH listener
  answers `/dns-query` and the public portal routes only; the admin namespace
  never reaches it.
- **An idle DoT connection is held for 60 seconds** (30 s on plain TCP), and there is no per-connection query limit. What a DoT client pays for a new connection is a full TLS handshake, so a phone on Android Private DNS that looks something up every few seconds used to re-handshake for nearly every name — tens of milliseconds, on mobile data, on the query that starts a game.
- **The reply tells the client how long it may hold the connection** (`edns-tcp-keepalive`, RFC 7828), so it does not have to guess.
- **Concurrent connections are capped** (512 TCP / 1024 DoT / 512 HTTPS) with header and idle timeouts on the HTTPS listener, so connection reuse cannot be turned into a way to hold the daemon's resources open.
- **The racer also speaks encrypted upstreams (a DigitDNS addition over upstream): DoT, DoH, DoQ, DoH3 and DNSCrypt.** An upstream entry may be a plain `1.1.1.1` (UDP, unchanged), `tcp://host:53`, `tls://host` (DoT, default port 853), an `https://host/dns-query` URL (DoH, RFC 8484, default path `/dns-query`), `quic://host` (DoQ, RFC 9250, default port 853, alias `doq://`), `h3://host/dns-query` (DNS-over-HTTP/3, default path `/dns-query`, alias `doh3://`), or an `sdns://…` DNS stamp (DNSCrypt plus stamped DoH/DoT). Each scheme is queried on its own transport and benchmarked the same way, and every encrypted transport verifies the upstream's certificate — there is no `InsecureSkipVerify` escape hatch. Add one from Dashboard → **Upstream Racers**.

### 📜 5. Certificates Issued by the Daemon
- **An embedded ACME client (RFC 8555) replaces certbot and acme.sh entirely.**
  Issuance and daily renewal happen **while the service runs** — the HTTP-01
  challenge is served out of the port-80 listener that is already running, so
  there is nothing to install and nothing to stop the service for. Renewed
  certificates hot-swap into the live listeners without a restart.
- **Three certificate surfaces, three names:** the panel, the subscription
  portal, and the DoT/DoH transports each carry a certificate validated against
  its own domain before anything persists — one wrong name is refused, not
  served.
- **The DoH/DoT transports can carry a dedicated certificate** (the custom
  DoH/DoT domain, e.g. for Android Private DNS), hot-swapped on issue, while
  everything else rides the panel certificate. The boot path probes the ACME
  store (`certs/acme/<domain>.crt`) like every other listener, so DoT and DoH
  come up on a migrated install without manual cert juggling.

### 📥 6. Bulk Downloads Stay Direct

A game unblocks a whole publisher: the store, sign-in, matchmaking, patch metadata — and the depot hosts that carry the actual multi-gigabyte payload. Those last ones are the only part of a gaming DNS that is measured in gigabytes rather than kilobytes, and relaying them is almost never what you want.

So this build resolves them directly, out of the box. Discord voice, Vivox voice, Steam's connection manager and Google's update CDNs (`dl.google.com`, `gvt1.com`, `gvt2.com`) sit on the built-in **forced-direct** plane, ahead of every rule, so a download like a Chrome or Android SDK package resolves to its **real** address rather than the VPS. Steam still opens, the library still loads its art, the game still connects — the download itself just runs on the subscriber's own connection.

If you *do* want one CDN relayed — for a subscriber whose line cannot reach it at all (a hard block, not a throttle: the symptom is a download stuck at 0 B/s while the store page loads fine) — add that host to **Custom Proxied**. A custom rule outranks the forced-direct plane for the name and its subdomains, so one host can be rescued without opening a whole category. Two mechanics worth knowing: the SNI proxy caps at 4096 concurrent relays and a download client opens dozens of parallel connections per install, so a relayed download spends that budget far faster than gameplay; and after changing a rule, flush the cache (`POST /api/v1/cache/flush`) or already-cached answers keep their old routing.

---

## 🔒 First-Run Hardening

Four settings decide whether this install is a private resolver or an open one. The defaults target a personal box on a home network — and the resolver ships **closed** (whitelist by default): the rows below say which ones still need changing before you let anyone else near it.

| Setting | Ships as | Change it to | Why |
| :--- | :--- | :--- | :--- |
| `access.allow_all` | `false` (whitelist by default) | keep it `false` to sell or share; `true` is for a personal all-devices install | While it is `true` the daemon answers every address, so disabling or expiring an account only stops it being *recognised* — the address is then served as anonymous `Public`, which has no plan to expire and no quota to exceed. Traffic limits still bite on accounts it knows, but **revocation does not work at all** until this is off. Fresh installs ship whitelist-on; an existing install converges through a one-time migration on first boot. |
| `security.api_bind` | `127.0.0.1` | leave it, unless a bot runs off-box | `127.0.0.1` means the REST API answers only the server itself and a remote call gets `403` however valid its key is. `0.0.0.0` puts an admin surface on the internet with the API key as the only thing in front of it. Prefer an SSH tunnel to flipping this. |
| Admin password | randomly generated, printed **once** in the install output | your own, ≥10 characters | It is stored as a PBKDF2-HMAC-SHA256 verifier (600,000 iterations) and never in the clear, so it cannot be read back out of `data.db` even with `master.key` in hand — which also means a lost password is a reset, not a recovery. Changing it invalidates every existing dashboard session. |
| `master.key` | generated on first start | back it up, off the server | It decrypts every subscriber record. Losing it makes them unreadable; leaking it makes them readable. It must never enter a git repository — `.gitignore` already covers it. |

An install carrying a password from before the strength policy keeps working — verification never applies the policy, so nobody is locked out of their own server by an upgrade — but it is flagged, and the dashboard asks for a replacement. New passwords must be at least 10 characters and combine two character classes, or be at least 16 characters, or use a non-ASCII script; a non-Latin passphrase satisfies the rule on its own.

Two things this build does *not* do, so you do not have to configure them: query logs are never written to disk in any form (DNS telemetry lives in memory and on the SSE stream), and the SNI relay never terminates TLS, so it never holds a key for the sessions it carries.

### 🕶 The Hidden Panel

The dashboard no longer lives at the root of the host. On first start the daemon generates a random **16-hex-character admin path** and mounts everything administrative below it — `/<admin-path>/dash/…` for the pages, `/<admin-path>/api/…` for the APIs — and prints the login address **once** to the log. The root domain serves an empty Matrix-style landing page; old panel routes answer the same 404 a random path gets, with no redirect to hint at the namespace. The path is a locator, not a credential, but treat it as one: regenerate it from Settings → *Hidden Admin Path* (every session and bookmark dies with it) if it has leaked anywhere, and never paste it into public docs.

Optional hardening on top, all in Settings:

- **Two-Factor Authentication** — RFC 6238 TOTP. Enroll from Settings → *Two-Factor Authentication*: the secret is shown exactly once, one code confirms it, and from then on logins (plus every password/API-key change) require a current code. Disabling demands both factors one last time.
- **Session idle window** — revoke a dashboard session after N minutes without use (5–1440, default 15). Applies live on save; the 24-hour absolute ceiling is unchanged.
- **Login lockouts** — 3 failures from one address locks it for 10 minutes. Clear it without a restart from TUI entry `[14]` or `POST /api/auth/unlock` (session or API key, plus a TOTP code when 2FA is on).
- **Subscription Portal** — the subscriber pages (`/sub/<token>`, `/ip/<token>`, and the dedicated listener if you set a port) stay public by design; Settings controls the title, the domain and port their links advertise, and an optional custom stylesheet that is sanitised (no `@import`, no external `url()`, no script) before it reaches a subscriber's browser.

For the full threat model, the exact list of encrypted fields, and a
report-a-vulnerability process, see **[SECURITY.md](docs/SECURITY.md)**.

---

## 🖥️ Interactive TUI Controller (`digitdns`)

Run DigitDNS in terminal mode to read live metrics, provision subscribers and run diagnostics over SSH without opening a browser:
```bash
./digitdns
# or, if installed system-wide (the installer symlinks it):
digitdns
```

It is a **numbered menu**, not a full-screen keyboard interface: it prints the banner with this instance's public IP, dashboard URL and API bind, then waits for a number and <kbd>Enter</kbd>. Everything is redrawn each time round, so a value the dashboard changed while you were reading is correct on the next pass.

| Option | Action | | Option | Action |
| :---: | :--- | :---: | :---: | :--- |
| `1` | Service status & telemetry | | `2` | Diagnostics |
| `3` | Restart the service | | `4` | Stop the service |
| `5` | Start the service (after a stop) | | `6` | List subscriber accounts |
| `7` | Add a subscriber | | `8` | Delete a subscriber |
| `9` | Benchmark DNS upstreams | | `10` | Flush the DNS cache |
| `11` | Change the dashboard panel port | | `13` | View / rotate the API key |
| `14` | Clear login lockouts | | `15` | Change admin credentials |
| `16` | Emergency admin reset | | `17` | Uninstall DigitDNS |
| `0`, `q` or `exit` | Quit (daemon keeps running) | | <kbd>Ctrl</kbd>+<kbd>C</kbd> | Quit at any prompt |

Options `3`–`5` and `17` reach **runtime ownership**: the console is a client
process that runs with the daemon down, so it is exactly where an operator
lands when the service is stopped and needs the way back up. Uninstall demands
a typed `UNINSTALL` confirmation. Option `14` calls the daemon's authenticated
unlock endpoint over loopback, so an operator who has locked themselves out
clears it without a restart. Option `13` prints the master API key in the
clear — treat that screen the way you would treat the key itself; rotation
stops the previous key the moment it lands, with no grace period.

Domain rules are **not** edited from here — that is the dashboard or the
REST API.

---

## 🌐 Web Administration Dashboard

The dashboard is not at the host root. On first start the daemon prints its one
**hidden login address** to the log — `https://<domain>/<admin-path>/login` — and
that is the only URL that reaches it. Bookmark it then; the path is a random
16-hex string, and the address is printed exactly once. To recover it later: the
TUI banner shows the dashboard URL, `digitdns status` reports listener and
certificate state without opening the database, and the log line is printed again
on every start.

- **Login is a standalone page** (`/<admin-path>/login`): it loads no dashboard
  assets, so the panel bundle is not readable before authentication. The
  two-factor field appears only when the password verified and the code is what
  failed — a plain wrong password never shows it.
- **Credentials:** Username `admin`. The installer generates a random password
  and prints it **once** in the install output; if you start the binary directly
  without a config it generates one and prints it in the startup log. There is no
  shared default password.
- **Dashboard tab:** live QPS chart, cache hit ratio, SNI-proxy counters, CPU/RAM,
  traffic meters, latency percentiles, and the **Upstream Racers** card (add
  upstreams on any transport, per-upstream latency, benchmark button).
- **Proxied Domains tab:** per-domain on/off switches, add box with **bulk
  import**, a sort selector (Added / Name / Status), a batch-size selector, and
  checkbox selection with bulk Pause / Resume / Remove.
- **Clients tab:** whitelist-mode toggle, search, add/edit modals (plan expiry,
  traffic limits, max devices, allowed IPs, **registration secret**,
  **Telegram provisioning card**), renew, suspend, reset traffic, regenerate UUID.
- **Live Query Log tab:** real-time SSE stream with search, action filter and
  pause — and per-row quick actions: one click on a query's ACTION badge offers
  Proxy / Block / Direct / Remove for the exact host, its `*.host` wildcard, or
  its `*.parent` wildcard.
- **API tab:** master API key management, network binding, and copy-paste
  integration snippets (cURL / Python / Node.js).
- **Settings tab:** two-factor authentication, hidden admin path, subscription
  portal (title, domain, port, custom CSS), administrator credentials, SSL/TLS
  manager (ACME issue/renew for panel, portal and dedicated DoH/DoT domain),
  blocked domains, DoH access tokens, static records with bulk import, custom
  policy groups with icon uploads, session idle timeout, and the About & Legal
  card crediting upstream HyperDNS.
- **Connect Guide tab:** copyable setup steps for Windows, PlayStation, Xbox,
  Android Private DNS and DoH clients.

### 🎨 Branding the Subscriber Portal (custom CSS)

Under **Settings → Subscription Portal → Portal custom CSS** the portal can be
themed three ways: **Inline** (paste CSS), **Local file** (an absolute path on
the server, read-only, ≤ 256 KB), or **URL** (an `http(s)` stylesheet the
visitor's browser loads directly — the panel's CSP allows exactly the configured
origin). Inline and local CSS are sanitised: `<style>`/`<script>` breakouts,
`@import`, external `url()` and `expression()` are stripped, so only your
geometry and colours survive.

```css
/* 1. Inline — brand the whole page with two colours and a font stack */
body.portal { background: #0b1220; color: #e2e8f0; }
.card { border-color: #00f0ff55; border-radius: 16px; }
.title { font-family: "Vazirmatn", sans-serif; letter-spacing: .02em; }
```

```css
/* 2. Inline — embed a logo with no external request (data URI) */
.brand-mark::before {
  content: "";
  display: inline-block;
  width: 96px; height: 32px;
  background: url('data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=') no-repeat center / contain;
}
```

**Group icons:** each named custom policy group can carry an icon from the group
editor — **SVG ≤ 16 KB or PNG ≤ 64 KB (≤ 512×512)**. The upload is re-encoded
through an allowlist on the server (scripts, handlers and external references
cannot survive) and is stored in the encrypted database, so your data backup
carries it automatically. Full walkthrough: [TUTORIAL.md → Brand the subscriber portal](docs/TUTORIAL.md#5c-brand-the-subscriber-portal-with-custom-css).

---

## 🔌 Developer REST API

DigitDNS exposes a developer-first REST API for bots and billing systems, in
two versions:

- **v2** (the successor contract, `/api/v2/…`): symmetric field names
  (`display_name`, `allowed_ips` — create and update finally agree),
  cursor-paginated lists (`{items, next_cursor}`, bounded limit), RFC 9457
  `application/problem+json` errors, `POST /clients/{id}/actions/{action}` for
  mutations, and **strict decoding** — an unknown field is a 400, not a
  silently dropped typo.
- **v1** (working, deprecated): every v1 route carries `Deprecation` and
  `Sunset` headers with a `Link: successor-version` pointing at v2. The master
  REST key (or a live dashboard session) authorizes both surfaces; it is
  **refused on every dashboard admin route** (scoping) except the
  lockout-recovery unlock.
- **Interactive Swagger Docs:** `/<admin-path>/api/v1/docs` — press
  **Authorize**, paste the Master API Key from Settings, and every endpoint
  becomes runnable from the browser. No key is needed to open the page itself.
- **The viewer ships inside the binary.** `web/swagger/` (Apache-2.0) is
  embedded and served same-origin, so the docs page works on a network that
  cannot reach a public CDN. "100% offline" is literally true for every asset.
- **Comprehensive API Specification:** See [API.md](docs/API.md).

**Quick samples** (replace `$KEY` with your Master API Key, `$HOST` with `IP:panel-port`):

```bash
# Create a subscriber (API v2) — the response carries the portal link + secret
curl -s -X POST "https://$HOST/api/v2/clients" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"display_name":"Ali","allowed_ips":["185.10.20.30"],"validity_days":30}'

# Check whether a newer DigitDNS release is published
curl -s "https://$HOST/api/v1/version"  # the running build
curl -s "https://$HOST/api/update/check" -H "Authorization: Bearer <dashboard-session-token>"

# Apply it from the CLI (the bundle must carry a valid ed25519 signature
# over checksums.txt AND a matching SHA-256 before anything is installed)
curl -s -X POST "https://$HOST/api/update/apply" -H "Authorization: Bearer <dashboard-session-token>"

# Upload an icon for a custom policy group (SVG ≤ 16 KB or PNG ≤ 64 KB)
curl -s -X PUT "https://$HOST/api/custom-groups/$GROUP_ID/icon" \
  -H "Authorization: Bearer <dashboard-session-token>" \
  --data-binary @my-icon.svg
```

---

## 🏛️ Technical Architecture & Codebase Design

A query walks four layers, in this order:

1. **Access control** — `IsIPAllowed(clientIP)` is a single map read behind a `sync.RWMutex`. Four indexes (`idMap`, `ipMap`, `tokenMap`, `uuidMap`) are rebuilt from BoltDB on change and re-filed one account at a time on a subscriber IP registration, so the public `/ip/{token}` endpoint never triggers a full rescan. Under `access.allow_all: true` an address that matches no index is served as anonymous `Public` rather than refused — see the warning in [First-Run Hardening](#-first-run-hardening).
2. **Cache** — a 64-shard LRU keyed by name+type+class. A hit returns a *copy* of the stored message, because the caller has to rewrite the transaction ID and count the TTLs down.
3. **Rule match** — an exact-match map plus a longest-suffix wildcard index over the operator's custom proxied, blocked and direct lists and any named groups. Longest suffix wins, so `cdn.example.com` can be routed differently from `*.example.com`.
4. **Action** — `proxy` spoofs the answer to this server so the SNI relay picks the connection up; `direct` races the configured upstreams and forwards the unaltered reply; `block` sinkholes it.

Measured on a **13th Gen Intel i5-13420H (12 threads), windows/amd64**, `go test -bench`, `-count 3`:

| Operation | Serial | 12 threads | Allocations |
| --- | --- | --- | --- |
| Subscriber lookup (`IsIPAllowed`, hit) | 16.8–17.6 ns | 30.1–32.1 ns | 0 |
| Subscriber lookup by UUID | 15.6–15.8 ns | — | 0 |
| Rule match, exact hit | 52.3–52.6 ns | 72.1–72.3 ns | 0 |
| Rule match, wildcard hit | 95.7–115.1 ns | — | 0 |
| Rule match, 600 extra admin rules | 132.8–142.8 ns | — | 0 |
| Cache hit | 258–302 ns | 127–133 ns | 6 / 296 B |
| Cache miss | 69.6–77.4 ns | — | 1 / 48 B |

**What these numbers do and do not say.** They are in-process costs for one layer each, not end-to-end query latency — the wire, the kernel, and (on a cache miss) the upstream dominate anything measured here. The index is guarded by one reader-writer mutex, so it does not scale with cores: the parallel figure is *worse* per lookup than the serial one, which is the expected shape for shared-lock reads and is fine at DNS request rates. The cache does scale, because it is sharded. A cache hit is **not** allocation-free — the defensive copy is the 6 allocations. Serial nanosecond figures on this class of CPU move by tens of percent between runs, so treat the ranges as ranges; the durable claims are the shapes (allocation-free matching, cost independent of how many subscribers or rules exist), not the third significant digit.

**Storage.** One BoltDB file. Secret fields — subscriber name, bound IP list, subscription token, REST API key, TLS paths — are individually sealed with AES-256-GCM under the 32-byte key in `master.key`; tokens additionally carry a keyed blind index so `/ip/{token}` stays one lookup. The admin password is never stored, only a PBKDF2-HMAC-SHA256 verifier (600,000 iterations) — so it cannot be read back out of `data.db` *even with `master.key` in hand*; a lost password is a reset, not a recovery. Traffic counters are accumulated in memory and flushed in a single transaction every 30 seconds, because every BoltDB commit is an fsync.

Deliberately in the clear: the **stored rule lists** (no credential, no subscriber name), and the BoltDB structure itself. **Query logs are never written to disk** in any form — DNS telemetry lives in memory and on the SSE stream only.

Keep `master.key` backed up and off the server. Losing it makes every subscriber record unreadable; leaking it makes them readable — it must never enter a git repository, and `.gitignore` already covers it.

---

## 🏆 Quality Scorecard

Scores are self-assessed against Google's guidance and the Mantis audit trail,
with every claim tied to something verifiable — the benchmark table above, the
named regression tests in [SECURITY.md](docs/SECURITY.md), and the live
external-attacker campaign this build went through. Not a Lighthouse run; a
panel that requires authentication has no public page to light up.

| Dimension | Score | Basis |
| :--- | :---: | :--- |
| 🔐 **Security** | **93 / 100** | Three full Mantis audit rounds + a fix-verification pass — **50+ findings, every one fixed with a named regression test** (master-key scoping, TOTP single-use, DoH surface restriction, port-drift, safe installers). A T3MP3ST-style external-attacker campaign against a live deployment found every headline control holding and produced one LOW, fixed live. Race detector: 20/20 packages, zero races. Deductions: the open-resolver posture remains one toggle away (documented non-goal), no WAF or volumetric DDoS layer (the provider's job), the admin path is defence-in-depth only. |
| ⚡ **Performance** | **90 / 100** | The hot paths are allocation-free nanosecond operations (subscriber lookup ~17 ns, exact rule match ~52 ns, cache hit ~258 ns — benchmarked, see Architecture), the cache is 64-shard concurrent, DoT holds idle connections (RFC 7828) instead of re-handshaking, relays draw from a `sync.Pool`, and the whole thing is one zero-CGO binary. Deductions: the subscriber index sits behind one RWMutex and does not scale with cores (documented shape), no published end-to-end wire latency numbers, the v1.2.0-era field reports (CPU pegged, memory leak) are fixed but the memory of them is why this is not a 95. |
| 🔎 **SEO** | **70 / 100** | Deliberate and honest: the public landing page serves `noindex, nofollow` **by design** — a private resolver's landing page has no business ranking, and the subscriber portal carries per-request `Vary` headers so no cache serves one subscriber's page to another. What Google's guidelines ask for structurally is there: real `<title>`, single `<h1>`, viewport meta, semantic HTML, mobile-first rendering. Deductions: no sitemap, no Open Graph/structured data, no indexable content — because the product is not a content site. Raise the score by removing one meta tag if you want the landing indexed. |
| 🧹 **Code hygiene** | **95 / 100** | gofmt/go vet clean across 20 packages, JS/Python/shell syntax gates, a markup↔JS consistency test that catches dead DOM lookups, changelog discipline, an offline-bundle mirror kept byte-identical, and multi-arch container images built from a locked-down `.dockerignore` and signed with keyless cosign. |

---

## 📐 Standards & RFC Compliance

DigitDNS implements and honours the following IETF specifications — the full
table, with what each RFC means in this project, lives in
[SECURITY.md → Standards & RFC Compliance](docs/SECURITY.md#-standards--rfc-compliance):

DNS core [RFC 1035/2181] · DNS-over-TCP [RFC 7766] · DNS-over-TLS [RFC 7858] ·
DNS-over-HTTPS [RFC 8484] · DNS-over-QUIC [RFC 9250] · DNS-over-HTTP/3
[RFC 9114] · DNSCrypt (via DNS stamps, `sdns://`) · Extended DNS Errors
[RFC 8914, verified live] · `edns-tcp-keepalive` [RFC 7828] · ACME [RFC 8555] ·
TOTP [RFC 6238] · HTTP semantics & HTTPS [RFC 9110] · problem+json [RFC 9457] ·
API Deprecation [RFC 9745] and Sunset [RFC 8594] · TLS SNI [RFC 6066] ·
UUID v4 [RFC 9562].

---

## ☕ Support / Donate

DigitDNS is a free and open-source fork of [HyperDNS](https://github.com/IzumiRain/HyperDNS).
If it unblocked your games or your work and you'd like to support ongoing
development of this fork, the addresses below reach the DigitDNS maintainer
directly.

| Method | Address / Link |
| :--- | :--- |
| 💰 **USDT · TRC20** | `TWrapnstisV3ZZsEgrfKDs2552SMc86VLE` |
| 💰 **USDT · ERC20** | `0xd52Edf3767080cfE3E7037b9bAd2f9Aa7a8833C4` |
| 💰 **TON** | `UQBV9O4TlKdnrqVqsbPD83ClBE_POBHHXD-HFYqWnqhvWDo` |

---

## 📜 License
DigitDNS is distributed under the **AGPL-3.0 License**, the same licence as the
upstream project it is forked from.

This program includes code from **HyperDNS** by HyperRAIN
([github.com/IzumiRain/HyperDNS](https://github.com/IzumiRain/HyperDNS)),
licensed under the GNU Affero General Public License v3.0. DigitDNS is a
modified fork; the original copyright notices are retained in full. See
[LICENSE](LICENSE) for the full licence text and [NOTICE](NOTICE) for the
attribution and modification summary.

Source code for this fork is available at
[github.com/digitrob/DigitDNS](https://github.com/digitrob/DigitDNS).
