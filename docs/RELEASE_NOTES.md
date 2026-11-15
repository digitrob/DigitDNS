# DigitDNS v5.8.16 — a quieter panel: faster filters, readable metadata, honest reads

A dashboard polish release on top of v5.8.15. No database schema migration is
required; existing settings, clients, blocklists, certificates, relay agents,
egress options and stored query history are preserved. The Android app is
unchanged apart from its version number.

## Changed

- **The list filters no longer rebuild on every keystroke.** The Domains search,
  the Add-domain box, the Live Log search and the Ad Blocking preset search each
  wiped and rebuilt their container on every `input` event. A 100-row live log is
  roughly 700 DOM nodes, so typing lagged once a list grew. All four are now
  debounced at 120 ms: a burst of six keystrokes produces one repaint, and the
  final filter still applies. The Users search was already debounced.

- **Every settings card says how its edits take effect.** Eighteen cards sat on
  one page and looked identical whether they saved on change, waited for the
  button at the bottom, or only took effect after a restart. Each now carries a
  badge on its title line — **Instant**, **Auto-saved**, **Save to apply** or
  **Restart required** — with the exact button named in the tooltip. Traffic
  History is the one genuine restart card; DNSSEC is "Save to apply", because its
  applier rebinds the running validator immediately.

- **The command palette is discoverable.** `Ctrl`/`Cmd`+`K` has worked since the
  palette was added, but nothing on the page said so. The sidebar now carries a
  search field with a `⌘ K` hint on Apple hardware and `Ctrl K` elsewhere, and the
  phone's More sheet carries the same entry — a phone has no shortcut at all.

- **Chart ticks are 11 px** instead of 10, with fewer ticks on a narrow screen, so
  the QPS and traffic-history axes stay legible on a phone.

- **Reduced motion is handled once, globally.** A single `prefers-reduced-motion`
  rule now covers every transition and smooth scroll instead of each component
  opting out individually. Spinners are deliberately excluded: they report
  progress rather than decorate.

## Fixed

- **A failed read no longer looks like an empty or default state.** Several
  loaders returned silently on a non-OK response, leaving a placeholder or a
  default that read as the server's real policy. The upstream resolver pool,
  custom policy groups, DNSSEC, Proxy Leak Protection, relay mode, query-log
  settings and alerts now show a failed-read row with a **Retry** button. The
  resolver card's Retry re-runs both requests that feed it, and a transient
  failure never wipes rows the operator can still use.

- **The resolver allowlist warns instead of showing an empty box.** Saving that
  field replaces the server's list with whatever it holds, so a failed read used
  to leave an empty box that would silently wipe a list the operator could not
  see.

- **The live log no longer claims to be connected while its polling reads are
  being refused.** The listening placeholder now says the recent queries could
  not be read, instead of "connected via polling".

- **The smallest metadata is readable.** 31 `text-[9px]` labels — stat captions,
  the sidebar IP block, the version badge — became 10 px on phones and 11 px on
  `sm+`, without disturbing the layout scale.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.16` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.16/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.16
docker pull ghcr.io/digitrob/digitdns-agent:5.8.16
docker pull robmeister/digitdns:5.8.16
docker pull robmeister/digitdns-agent:5.8.16
```
