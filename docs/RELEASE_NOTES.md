# DigitDNS v5.13.1 — AdGuard `$dnstype` rules and a QUIC fast-fail guard for the relay

A small, targeted patch. Two things that made a proxied name feel slow — or made a pasted rule do nothing — now behave.

## `$dnstype` rules are enforced

AdGuard's `$dnstype` modifier used to be treated as an unknown modifier, and the blocklist parser skips a line it cannot fully understand. A pasted rule such as:

```
||google.com^$dnstype=HTTPS
```

was therefore silently discarded. It now narrows a block — or an `@@` allow exception — to the named query types:

- `HTTPS`, `SVCB`, `A`, `AAAA`, `TXT`, any mnemonic, or `TYPE<n>`; pipe lists (`HTTPS|SVCB`) and any case work.
- Exact, wildcard, custom-regex and downloaded-list rules all carry the filter.
- `$dnsrewrite` combined with `$dnstype` is skipped rather than guessed at.

Blocking the HTTPS/SVCB record removes the `h3` hint without touching A/AAAA. Pair it with the **PROXY** action on the name and proxy leak protection so A points at the relay and AAAA/HTTPS stay sunk.

## QUIC fast-fail on the relay

The relay speaks TCP only, but a proxied name answers with the relay's address. A client that learned HTTP/3 from an HTTPS/SVCB record — or from an older Alt-Svc cache — opens QUIC over UDP 443 and waits for a handshake that can never complete.

The daemon now binds UDP 443 beside the HTTPS listener and answers a client Initial with a **Version Negotiation packet that offers only a reserved version**. The client has no acceptable version, abandons QUIC at once, and retries over TCP through the relay.

- Only datagrams of at least 1200 bytes are answered, so the guard cannot be used as a reflector; the reply is smaller than the trigger.
- Per-source and global token buckets bound the work; the source table is capped and swept.
- It binds only when the HTTPS TCP listener owns the port, so another process's HTTP/3 is never touched.
- Enabled by default. A host that runs a real HTTP/3 service on the same port can opt out with `"quic_reject": false` in the `sniproxy` config block. The image exposes `443/udp`.

## Counters in the panel and the app

`relays_quic_rejected`, `relays_quic_ignored` and `relays_quic_limited` are on the **SNI Proxy** card in the web panel and the Android dashboard, present in both the full and compact stats responses, and exported as `digitdns_relays_quic_*_total` for Prometheus. The line is hidden at zero so a TCP-only install keeps its card unchanged.

## Upgrade

- Server: pull the new image (`docker compose pull && docker compose up -d`) or use the one-liner / in-dashboard self-updater. If the relay runs behind a firewall that is not `ufw`/`firewalld`, allow **UDP 443** as well as TCP; the guard is inert without it, and QUIC clients keep their old timeout behaviour.
- Android: `android-v5.13.1`, version code 595.
- No database schema migration is required.
