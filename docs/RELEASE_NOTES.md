# DigitDNS v5.11.2 — macOS DNS profile fixes

This release adds a first-class way to hand a macOS client a working encrypted-DNS configuration. Both the dashboard and the Android companion can now generate a `.mobileconfig` profile from the server's own transport settings, so the exported endpoint can never drift from what the resolver actually listens on. No database schema migration is required: existing rules, saved sources, certificates, relay agents, egress options, stored history and service icons are preserved.

## Dashboard: DNS Transport Profiles

Under **Settings → Resolvers**, the new **DNS Transport Profiles** card sits directly after Client Transports, with its own `DNS Profiles` jump chip.

- Choose **DoH (DNS over HTTPS)** or **DoT (DNS over TLS)**.
- The server endpoint is derived live from the configured certificate domain (falling back to the public IP) and the configured DoH/DoT ports — 8443 for DoH with `/dns-query`, 853 for DoT. Port 443 is omitted from the DoH authority.
- **Generate .mobileconfig** downloads a named profile and reports the saved filename; the last chosen transport is remembered locally.
- The endpoint refreshes automatically whenever the configuration is reloaded or edited.

## Android: DNS Profiles in the settings hub

**DNS Profiles** is now a searchable entry under **Settings → Server & access**, next to Transports and Access — it is not a detached screen.

- It reads the same `ClientTransportsViewModel` state as the transports page, so host and ports match the server.
- DoH/DoT selection, a live endpoint readout, and a **Generate .mobileconfig** action that writes the file to the app cache and shares it through a `FileProvider` chooser (mail, Files, AirDrop, and so on).
- A short note in the panel explains installation and removal on macOS.

The companion carries the matching `android-v5.11.2` version line, version code 584.

## Profile contents

Both clients emit an unsigned `com.apple.dns.managed` payload with `DNSSettings`:

- `DNSProtocol` is `HTTPS` for DoH and `TLS` for DoT.
- DoH sets `ServerURL` to `https://host:8443/dns-query` plus `ServerNames` for certificate validation.
- DoT sets `ServerAddresses` and `ServerNames` to the configured host, on port 853.
- Each payload has its own fresh UUID, `PayloadVersion` 1, and `PayloadRemovalDisallowed` false so the user can remove it normally in System Settings → VPN & Device Management.

Because the profile is unsigned, macOS presents it as an unmanaged profile to approve manually; that is intentional for this self-hosted use case.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.11.2/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.11.2
docker pull ghcr.io/digitrob/digitdns-agent:5.11.2
docker pull robmeister/digitdns:5.11.2
docker pull robmeister/digitdns-agent:5.11.2
```
