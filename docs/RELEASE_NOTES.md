# DigitDNS v5.13.2 — the relay no longer trusts a single nameserver

A resolver-robustness patch. A proxied name could be refused even though the destination was reachable: the relay asked the system resolver, and Go's resolver waits up to five seconds on the first server in `resolv.conf` while the relay allowed the whole lookup two. One dead stub — a Tailscale node with DNS disabled, for example — or one lost UDP packet was enough.

## Every nameserver is queried at once

`internal/netresolve` reads the host's `resolv.conf` and sends the query to every server concurrently. The first usable answer wins and the rest are cancelled, so a silent or lossy server costs nothing and a normal lookup finishes in about 10 ms.

- Applies to the hub relay **and** the agent relay, which had its own copy of the sequential resolver.
- The panel's Tools check and blocklist URL validation use the same resolver; list validation also gained a five-second bound instead of an unbounded `net.LookupIP`.
- Start-up now names the resolver set:

  ```
  [SNI Proxy] Target resolution races 4 nameserver(s): 8.8.8.8:53, 1.1.1.1:53, 1.0.0.1:53, 9.9.9.9:53
  ```

## What it fixes

On a host whose `resolv.conf` began with a dead stub and included deprecated resolvers, a cache-miss lookup failed whenever the server Go happened to be waiting on dropped the packet. The relay then refused the connection with:

```
[SNI Proxy] Refused relay from …: proxy: resolving signaler-pa.googleapis.com: lookup …: i/o timeout
```

That intermittently broke proxied Gemini/NotebookLM endpoints such as `signaler-pa.googleapis.com` and `robinfrontend-pa.googleapis.com` while the same names worked through an agent on a host with a clean resolver.

## `resolve_servers`

Empty (the default) races the host's nameservers. To pin a list:

```json
"sniproxy": {
  "resolve_servers": ["8.8.8.8", "1.1.1.1"]
}
```

Bare addresses and `host:port` pairs are accepted; duplicates are dropped and every entry is queried concurrently.

## Upgrade

- Server: pull the new image (`docker compose pull && docker compose up -d`) or use the one-liner / in-dashboard self-updater.
- Android: `android-v5.13.2`, version code 596.
- No database schema migration is required.
