# DigitDNS v5.12.2 — Domains: attributes that survive a scope change, and no duplicate rules

A Domains-page release. Changing a rule's wildcard scope now keeps its service label, relay and resolution override; adding a domain that an existing rule already covers is flagged and skipped; the Custom Blocklist becomes a selectable group of its own; and every domain in Domains and the Live Log gets a copy button. No database schema migration is required.

## A scope change keeps the row's attributes

Changing a domain between **Exact**, **`*.host`** and **`*.parent`** rebases the stored pattern. Until now that rebase touched only the four routing lists: the service label, relay override and resolution profile stayed under the old key, the server pruned them, and the new wildcard row came back **with no service**. The same happened when an action (Proxy / Block / Direct) changed the pattern through a wildcard scope.

Both web paths (`applyStreamScope`, `applyStreamAction`) and the Android `RouteAction` / `changeScope` paths now move these attributes with the rule:

- `domain_services` and `domain_agents` (service label, relay override);
- the `Added` timestamp, so the row keeps its place in the sort;
- the per-domain resolution profile, saved after the rules write succeeds.

An entry already stored on the target wins, and a separate wildcard row's label is never taken. Removing a rule also drops its relay, stamp and resolution entry instead of waiting for the server to prune them.

## No duplicate rules when adding a domain

A plain rule covers itself and **every subdomain**; a wildcard covers its subtree. Adding `frontend.googleapis.com` when `googleapis.com` is already managed therefore created a rule that could never win.

- The Add box explains coverage **while you type**: `Covered by googleapis.com (PROXY) — adding this name is a duplicate…`.
- On Add, covered names are listed with **SKIP COVERED** (default) and **ADD ANYWAY** — the latter for the legitimate case of giving a subdomain its own service or relay label.
- Bulk import reports `added / already covered / invalid` and imports only the fresh names.
- Adding a **broader** rule (`googleapis.com` while `*.googleapis.com` exists) names the narrower rules it makes redundant. Nothing is deleted automatically.
- A new proxy name under a blocked or direct parent is added but reported as shadowed, because those lists outrank proxy in the matcher.

Android applies the same coverage check on its Add box and reports how many names were skipped.

## Select only the Custom Blocklist

The default Domains view folds the Custom Blocklist into one entry. Two ticks now have separate scopes:

- **Header Select all** selects everything in view **except** the blocklist (`Select all domains except the Custom Blocklist`).
- **The sinkhole row's checkbox** selects **only** the blocked domains in view — the whole set, not just the rendered batch — so a service or relay can be applied to that group alone.
- Ticking the group never expands it; the chevron still does.
- A search or a status chip flattens the view, and the header tick then includes matching blocked rows. A service filter narrows both scopes.
- Android gets the same split with a tri-state group checkbox.

## Copy any domain

**Domains** and the **Live Log** now show a small copy glyph after the domain name. It copies the exact stored pattern — including the `*.` prefix on a wildcard — flashes a green check, and is reachable from the keyboard. It does not select the Domains row and does not filter the Live Log.

## Verification

`go vet`, the full Go suite, the race detector, the dashboard regression tests, Android unit tests, detekt and Android lint pass. Web assets were rebuilt and their tests pass. No database schema migration is required.

The Android companion moves to **android-v5.12.2**, version code **592**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.12.2/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.12.2
docker pull ghcr.io/digitrob/digitdns-agent:5.12.2
docker pull robmeister/digitdns:5.12.2
docker pull robmeister/digitdns-agent:5.12.2
```
