# DigitDNS v4.0.2

A one-line follow-up to 4.0.1.

The agent's client allowlist can be the literal entry `any`. The status line
counted only explicit CIDRs, so an allow-all agent printed:

```
0 client entries
WARNING: empty client allowlist - every connection will be refused
```

while it was actually relaying normally. `Len()` now counts `any` as one rule,
so the startup line reads `1 client entry` and the false warning is gone.

No relay behaviour, catalogue sync, heartbeat, traffic accounting or
configuration changed.

## Upgrade

Replace the agent binary or pull `ghcr.io/digitrob/digitdns-agent:4.0.2` and
recreate the container, keeping `/data/agent-state.json`. Hub binaries and the
offline bundle for 4.0.2 are on this release as usual. Android remains 4.0.0.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `checksums.txt`, `checksums.sig`

Full list of changes: [docs/CHANGELOG.md](CHANGELOG.md).
