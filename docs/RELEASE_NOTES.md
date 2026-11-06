# DigitDNS v5.2.0 — DNSSEC validation, panel and Android controls

**A feature release.** 5.2.0 adds full DNSSEC validation to the resolver, a
Settings card and v3 API for managing it, and a dedicated DNSSEC section in the
Android companion. It is opt-in and startup-scoped: an existing install keeps
its current behaviour until `dnssec_validation` is enabled and the daemon
restarts.

There are no database schema changes and no breaking API changes.

## DNSSEC validation

- **Full chain-of-trust validation.** The hub walks from the built-in IANA root
  KSKs (`20326`, `38696`) through DS and DNSKEY records at each zone cut, then
  verifies the answer's RRSIGs.
- **Authenticated denial of existence.** NXDOMAIN and NODATA are checked with
  NSEC or NSEC3 proofs, including wildcard answers and NSEC3 opt-out spans.
- **Honest status bits.** `AD` is set only on answers this hub validated itself.
  Bogus answers return `SERVFAIL` with Extended DNS Error 6 (`DNSSEC Bogus`).
  `CD=1` bypasses validation and never claims `AD`. Unsigned delegations are
  served normally without `AD`.
- **Cache correctness.** Cache keys separate EDNS, DO and CD variants; DO=0
  responses never leak RRSIG, NSEC or NSEC3 records; background refresh keeps
  the entry's flags; cached TTLs are capped at the nearest RRSIG expiration.
- **Trust anchors.** Empty `dnssec_trust_anchors` uses the built-in root KSKs.
  Custom DS or DNSKEY anchors are validated before saving. The policy and
  anchors are startup-only and require a daemon restart.

## Configuration

```json
"dnssec_validation": true,
"dnssec_trust_anchors": []
```

`dnssec_validation` defaults to `false`. Existing installs keep passing the
upstream `AD` bit through until validation is enabled.

## Web panel

- **Settings → DNSSEC Validation**, placed directly above **Traffic History**.
- Enable or disable validation, edit root trust anchors, inspect the built-in
  anchors, and see the difference between saved and running policy.
- A restart-required banner appears when the saved record differs from the
  running engine. Saving is deliberately not a live toggle.
- New authenticated routes:
  - `GET/POST /api/settings/dnssec`
  - `GET/POST /api/v3/settings/dnssec` for the Android client and API-key
    integrations.

## Android companion

- **DNSSEC** section in Settings, immediately before **Traffic History**.
- The card opens with the saved state already loaded; the switch does not flip
  from a default value and the status pill does not change after opening.
- Trust-anchor editor, expandable built-in root anchors, restart-required
  notice, and the shared bordered status-badge style used elsewhere in the app.
- Compact layout: the card fits the Settings area without scrolling even with
  the built-in anchors expanded.

## Upgrade

The in-panel self-updater and the one-line installer use the same release.

```bash
docker pull ghcr.io/digitrob/digitdns:latest
docker pull ghcr.io/digitrob/digitdns-agent:latest
```

Docker Hub mirror:

```bash
docker pull robmeister/digitdns:latest
docker pull robmeister/digitdns-agent:latest
```

Pinned versions:

```bash
docker pull ghcr.io/digitrob/digitdns:5.2.0
docker pull robmeister/digitdns:5.2.0
```

## Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-agent-5.2.0-linux-arm64-image.tar.gz`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Android APKs are attached to this release from the `android-v5.2.0` tag.
