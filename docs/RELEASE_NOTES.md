# DigitDNS v5.0.1 — Dashboard polish, DNS Rewrites, and relay clarity

A focused patch on top of **5.0.0**. There are no database migrations and no
configuration changes: current installs can upgrade in place.

This release cleans up the dashboard, makes live-log statuses clearer, renames
static records to **DNS Rewrites**, and brings the Android companion closer to
the web panel.

---

## 🧩 DNS Rewrites

- **Pinned Records / Custom Static Records are now DNS Rewrites** on web and
  Android. The stored `custom_records` field and the API are unchanged.
- Android's **Pin** action is now **Save**.
- Android gains **Bulk import** for rewrites, matching web:
  - paste a table or choose a text file;
  - one `hostname address` pair per line;
  - spaces, commas, semicolons or `=` separators;
  - `#` comments, strict IPv4 and IPv6 validation;
  - updates existing hosts, counts unchanged/invalid lines and keeps the
    dialog open when some lines fail.
- Web and Android show the same **DNS rewrites: N added, N updated, N unchanged,
  N invalid** summary.

---

## 📊 Dashboard cards

- All Android dashboard card names now use the same style: **uppercase, muted
  label colour and the larger card type size**, with a matching pictogram.
- Added the missing pictograms to **Summary** and **Queries over time** on
  Android, and to the six history KPI cards on web.
- Android **Queries over time** labels now follow the selected window:
  - 1h/24h → times
  - 7d → weekday and date
  - 30d/90d → dates
  - four evenly spaced ticks instead of only first/last.
- The 1h chart spans the **full selected hour**; future five-minute space stays
  empty instead of drawing a misleading line to zero.

---

## 🛰️ Queries by Relay

- The **Relay Countries** card is now **Queries by Relay** on both surfaces.
- **Web:** flag, relay name, query count and share bar.
- **Android:** flag, relay name and query count; no bar.
- **Local** uses the installed server's country flag.
- Relay flags are sized to fit the row cleanly.
- Protocol names now use conventional casing — **DoH, DoT, DoQ, DoH3,
  DNSCrypt** — while UDP/TCP stay uppercase. The same protocol palette is used
  in the live log and history cards.

---

## 📱 Android header and live log

- **Live Log query count** sits in the top-left of the Updated/Online row.
  It uses the exact same 8sp shared meta style and a fixed-width slot, so it no
  longer drifts or overlaps.
- A DNS rewrite match now shows a **Rewrite** status badge in the live log
  (violet, same tappable action slot as Proxy/Direct/Block).
- **Domains Reload** is back on the left without resizing or restyling it.
- **Clients “+”** is back in the top-left.
- Domain pause/resume switches are visually thinner while keeping the full 48dp
  touch target.

---

## ⬆️ Upgrade

```bash
docker pull ghcr.io/digitrob/digitdns:5.0.1
docker pull ghcr.io/digitrob/digitdns-agent:5.0.1
```

Recreate the container as usual. Existing `/app/data` and `/app/certs` volumes
are reused. Android APKs are attached as `android-v5.0.1` (arm64-v8a,
armeabi-v7a, x86_64 and universal).

## 📦 Assets

- `digitdns-linux-amd64`, `digitdns-linux-arm64`
- `digitdns-agent-linux-amd64`, `digitdns-agent-linux-arm64`
- `digitdns-offline-bundle.tar.gz`, `digitdns-offline-bundle.zip`
- `digitdns-{arm64-v8a,armeabi-v7a,x86_64,universal}.apk` and `.sha256`
- `checksums.txt`, `checksums.sig`

Screenshots: [README gallery](https://github.com/digitrob/DigitDNS#screenshots).
Full changes: [docs/CHANGELOG.md](https://github.com/digitrob/DigitDNS/blob/v5.0.1/docs/CHANGELOG.md).
