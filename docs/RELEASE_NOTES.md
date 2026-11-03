# DigitDNS v3.8.0

This one is about making the panel fit the way you use it. The dashboard can now
be rearranged, and domains can carry service labels that help you find them
without pretending to be routing rules.

## Highlights

- **Arrange the dashboard yourself.** Click the pencil beside **Dashboard** in
  the sidebar, drag cards into the order you want, and pull the corner grip to
  resize them. The layout is saved on the server, so it is there the next time
  you sign in from another browser. On a phone, long-press a card to move it.
- **Services for domains.** Create labels such as *BBC*, *Work* or *Kids*,
  assign them from the Domains row or in bulk, and click a service to filter the
  list. Services never change whether a domain is proxied, blocked or direct.
- **Icons that mean something.** Use a domain's favicon, paste an image URL, or
  search a word like `tv` for a colourful icon. The panel proxies the Iconify
  search so the dashboard CSP stays closed to third-party scripts.
- **Bulk assignment.** Select several domains and use **Assign service** to
  label them at once, or pick **No service** to clear them.
- **Android keeps up.** Long-press to select domains, use the one-row selection
  bar for bulk actions, and assign services from a picker with the same labels
  and icons as the web panel.

## Under the hood

- The dashboard arrangement is stored as the `dashboard_layout` custom policy
  and exposed through `/api/config`; it is written by
  `POST /api/dashboard/layout`.
- Icon searches are cached per query and ranked with the colourful icon sets
  first. The dashboard never talks to the icon host directly.
- A partial rules save now preserves `services`, `domain_services` and
  `service_icons`; before this, toggling one domain could wipe every label.
- The old `hyperdns_token` and `hyperdns_theme` browser keys are no longer read
  or written, and the remaining inherited strings were cleaned up.

## Upgrading

There is no database migration and no configuration change. The dashboard
layout is created the first time you save one. Docker users can pull
`ghcr.io/digitrob/digitdns:latest` (or `3.8.0`); binary installs replace
`digitdns` and restart the service. The Android app is version 3.8.0
(versionCode 380) and updates independently.

Back up `data.db` and `master.key` before any upgrade.
