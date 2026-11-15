# DigitDNS v5.8.15 — quiet switches, a calm live log

A UI polish release on top of v5.8.14. No database schema migration is required;
existing settings, clients, blocklists, certificates, relay agents, egress
options and stored query history are preserved.

## Fixed

- **Server-backed switches no longer animate when a page opens.** They were
  rendered in the markup as off and corrected only after the settings fetch
  returned, so every visit replayed an OFF→ON slide. The six affected switches —
  Enable ad blocking, Relay mode, query-log Enabled, alerts Enabled,
  Public/Whitelist mode and Single upstream resolver — now start hidden with
  their space reserved and are revealed already carrying the saved value. A
  failed read reveals the markup state instead of leaving an invisible control.
- **The live-log status pill no longer flickers or shifts the toolbar.** Every
  dropped EventSource used to paint three labels in a row — `Reconnecting…` from
  the error, `Polling` from the REST fallback, `Live` from the 1-second
  reconnect — and because the label changes width, the Pause/Clear buttons moved
  with it. Degraded states are now delayed: the current label holds through the
  first backoff, `Reconnecting…` appears after 1.5 s, and `Polling` only after
  12 s if the fallback is actually carrying the log. Any delivered frame restores
  `Live` in one step. The pill also has a fixed width for every label.
- **An idle stream is no longer labelled `Polling`.** Opening the Logs tab, or
  returning to the browser, used to start the REST fallback whenever no query had
  arrived for three seconds — even though the EventSource was open and healthy.
  The fallback now starts only when the socket is not open; the buffered-stream
  check at connect time still detects a proxy that swallows the stream.
- **Android: the live-log connection chip moved to the right edge of the filter
  row.** It used to sit at the end of the scrolling Reload/Sort/Pause/Clear row,
  which put it off-screen on a phone exactly when it mattered. The action chips
  scroll; the status stays pinned and visible.

## Changed

- **The AdGuard rules field grows with the window.** The Ad Blocking page's left
  column ended in dead space under the rule tester while the blocklist card ran
  to the bottom. The policy card now fills that height and the Custom filtering
  rules textarea absorbs it (minimum 9 rem, still resizable), so both columns end
  level at any window height. On a short window the card keeps its content height
  and the column scrolls instead of compressing the fields.

## Upgrade

In-place upgrades from the dashboard are supported. The release assets include
the Linux amd64/arm64 hub binary, relay-agent binaries, the offline bundle,
checksums and their ed25519 signature, and the Android APKs attached from the
`android-v5.8.15` tag.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.8.15/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.8.15
docker pull ghcr.io/digitrob/digitdns-agent:5.8.15
docker pull robmeister/digitdns:5.8.15
docker pull robmeister/digitdns-agent:5.8.15
```
