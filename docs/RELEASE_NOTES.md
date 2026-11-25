# DigitDNS v5.12.0 — A new identity and a home-screen icon you choose

DigitDNS has a new mark across the panel, login page, PWA, Android app and documentation. The Android companion also lets you choose the home-screen icon — **Regular** or **Grayscale** — and this release carries the cold-miss coalescing fix from `main`. No database schema migration is required.

## A new mark

The new tile reads as a DNS switch with a globe at the end: encrypted resolution, routing and the public internet in one shape. It replaces the old logo everywhere it was visible:

- **Web panel:** sidebar and mobile header, favicon set (ICO, SVG, 16/32/64 px PNG), Apple touch icon, and PWA icons at 192/512 px plus maskable variants.
- **Login page:** the brand lockup and both inline favicons.
- **Android:** adaptive colour icon, adaptive grayscale icon, monochrome layer for themed icons, and a white notification glyph generated from the same mark.
- **Documentation:** README hero, Android screenshots and the GitHub repository social preview.

The browser manifest, theme colour and Open Graph metadata now point at the new assets. The social card is 1280×640, uses stronger text contrast and lists **Smart DNS · SNI Proxy · Ad Blocking**.

## Choose the app icon

**Settings → App → App icon** now shows the two real icons instead of text chips. Pick one and the launcher entry switches in place:

- **Regular** uses the full-colour adaptive icon.
- **Grayscale** uses a desaturated version of the same mark.
- **Themed icons** follow Android's monochrome layer in either mode.
- The choice is stored in DataStore, restored at launch, and normalised on every start so an interrupted switch cannot leave both aliases enabled or disabled.
- No reinstall is needed. Some launchers cache the icon for a moment or until their next restart.

The launcher entries are two activity aliases whose names are built from the application ID. That last detail matters: the previous implementation named them from the manifest namespace, so a debug build saved the preference but silently failed to change the icon.

## Force switches on the first tap

The Force button now selects from the latest resolver measurements immediately. It no longer waits for a full three-sample benchmark, and it does not queue behind a scheduled one; a pool that has never been measured gets a single short probe first. The Android Force response also returns the ranked upstream rows, so the card applies the new resolver and its latency in one round trip instead of reading a cached follow-up snapshot.

## Faster cold paths

A client that arrived just after the leader's upstream answer but before the handler wrote it to the cache could still start a second query for the same name. Completed single-flight lookups are now reused for **500 ms**, bounded to 4,096 entries and swept when full. A busy resolver sees one upstream query per cold name instead of a small burst.

## Screenshots and docs

Every README screenshot was recaptured in a synthetic local environment: dashboard in Dark, Grey and Light, Domains, Ad Blocking, Traffic History, the subscriber portal, and the Android app in all three themes plus App settings. The screenshots use documentation addresses and demo client names only; the disclaimer says so.

## Verification

`go vet`, the full Go suite, the race detector, the dashboard regression tests, Android unit tests, detekt and Android lint pass. The debug APK assembles locally; CI builds and signs the release APKs.

The Android companion moves to **android-v5.12.0**, version code **590**.

## Upgrade

In-place upgrades from the dashboard are supported.

```bash
curl -fsSL https://raw.githubusercontent.com/digitrob/DigitDNS/v5.12.0/scripts/install.sh | sudo bash
```

Docker:

```bash
docker pull ghcr.io/digitrob/digitdns:5.12.0
docker pull ghcr.io/digitrob/digitdns-agent:5.12.0
docker pull robmeister/digitdns:5.12.0
docker pull robmeister/digitdns-agent:5.12.0
```
