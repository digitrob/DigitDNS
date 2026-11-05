# DigitDNS v5.1.0 — Graphite theme for web and Android

**A theme release.** 5.1.0 adds **Graphite**, a neutral third theme for the web
panel and the Android app, alongside Dark and Light. One button cycles
**Dark → Grey → Light**; the choice is stored locally, survives reloads, and
carries through the login screen, sign-in and app restarts.

This is a drop-in upgrade. There are no schema, API or configuration changes.

## Web panel

- **Graphite theme** in the sidebar theme button. The icon shows the current
  theme: moon, half-filled circle or sun.
- **Neutral selected states** for filter chips, range controls, API tabs,
  service rows, live-log scopes and the agent architecture selector.
- **No cyan on large surfaces.** Primary buttons use a light-grey fill with
  dark text; soft and ghost buttons are neutral; card top edges, hover borders
  and shadows are grey.
- **Cyan remains only as a small accent** on icons, links, the chart line and
  focus rings.
- **Login screen and standalone login** follow Grey and Light consistently,
  including field borders, placeholders and focus rings.
- **Accessibility fixes**: stronger field boundaries, neutral selection, and
  underlines on links that no longer rely on a colour cue.

## Android companion

- **Grey theme** in **Settings → Theme**, next to Follow system, Dark and Light.
  The four chips wrap on narrow screens.
- **Theme button on the login screen** cycles Dark → Grey → Light. The choice is
  persisted through `AppPreferences`, so it survives sign-in, process death and
  relaunch.
- **Grey launcher icon** and a matching grey in-app shield mark.
- **No cyan highlights** in buttons, chips, switches, checkboxes, selected role
  segments, card borders or progress indicators.
- Semantic green, amber and red stay; DoH uses a soft lavender accent so
  protocol chips remain distinguishable.

## Screenshots

The README screenshots section now includes the Graphite web dashboard and the
Android login, dashboard and theme settings. Public addresses are replaced with
documentation addresses or pixelated; no client names are shown.

## Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

If you pin a version:

```bash
docker pull ghcr.io/digitrob/digitdns:5.1.0
docker pull ghcr.io/digitrob/digitdns-agent:5.1.0
```

The in-panel self-updater and the one-line installer use the same release.

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.1.0` tag.

Full changes: [docs/CHANGELOG.md](https://github.com/digitrob/DigitDNS/blob/v5.1.0/docs/CHANGELOG.md).
