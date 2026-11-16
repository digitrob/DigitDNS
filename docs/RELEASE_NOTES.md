# DigitDNS v5.8.17 — the quick actions save again

A one-bug fix release on top of v5.8.16. No database schema migration is
required; existing settings, clients, blocklists, certificates, relay agents,
egress options and stored query history are preserved. The Android app is
unchanged apart from its version number.

## Fixed

- **Changing a domain's routing from the domain list works again.** Clicking a
  row's status badge opens the shared quick-action menu — exact host, `*.host` or
  `*.parent`, then **Proxy**, **Block** or **Direct**. Since v5.8.12 that menu has
  done nothing: the handler read a constant that had been deleted while the live
  log's info button was added, so it threw a `ReferenceError` on every click.

  The throw landed in the worst possible place. The rule lists had already been
  edited in memory, and the save request had not been sent yet, so:

  - the row kept its old status badge, which read as "the click was ignored",
  - nothing reached the server, so the change was never real,
  - the in-memory edit was discarded the next time the panel loaded config.

  It affected every domain in every list, on the Domains page **and** on the Live
  Log's ACTION column, because both use the same handler. A service label — the
  ChatGPT bundle in the report that found it — had nothing to do with it; labels
  are organisational and never touch routing.

- **A lookup like that can no longer abort a save.** The badge map is now read
  before the rule lists are touched, and the live-log buffer rewrite is
  best-effort. If either ever fails again, the rules are still written and the
  operator still gets a toast.

## Changed

- **The `exact` match scope says what it does.** Its hint read "host + subdomains"
  while the scope writes the bare name and drops any host wildcard the row also
  owned. It now reads "this host only".

## Build

- **A stray closing brace in `style.css`** is gone. esbuild warned about it on
  every asset build; the rules after it were already parsed at the top level, so
  nothing on the page changes.

- **A new test guards against the same shape of bug.** `node --check` cannot see a
  reference to a constant that no longer exists — that is a runtime
  `ReferenceError`, not a syntax error, and it only appears in the console of
  whoever clicks the control. `tests/web/js-globals.test.cjs` strips comments and
  string literals from `app.js`, collects every declared name, and fails on any
  `SCREAMING_CASE` identifier that is used but never declared.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.17` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.17/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.17
docker pull ghcr.io/digitrob/digitdns-agent:5.8.17
docker pull robmeister/digitdns:5.8.17
docker pull robmeister/digitdns-agent:5.8.17
```
