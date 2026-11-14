#!/usr/bin/env bash
# Regenerate the minified dashboard assets that are served under the original
# /js/app.js and /css/style.css URLs. The source files stay embedded and readable
# for the Go tests; only the bytes served to browsers are minified.
#
# The minified file carries a sha256 of its source in a banner. A Go test compares
# that banner with the current source, so editing app.js or style.css without
# re-running this script fails the test suite instead of silently shipping stale
# minified bytes.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

JS_HASH="$(hash_file web/js/app.js)"
CSS_HASH="$(hash_file web/css/style.css)"
HTML_HASH="$(hash_file web/index.html)"

TMP_JS="$(mktemp)"
TMP_CSS="$(mktemp)"
TMP_HTML="$(mktemp)"
trap 'rm -f "$TMP_JS" "$TMP_CSS" "$TMP_HTML"' EXIT

npx --yes esbuild web/js/app.js --minify --charset=utf8 --target=es2020 --outfile="$TMP_JS" >/dev/null
npx --yes esbuild web/css/style.css --minify --charset=utf8 --target=es2020 --outfile="$TMP_CSS" >/dev/null

# Minify the dashboard document. Inline scripts are deliberately NOT minified:
# the CSP pins their SHA-256 hashes, and rewriting their bytes would make the
# browser refuse the pre-paint theme/route blocks. minified_assets_test.go checks
# that the served document still carries the same inline-script hashes.
npx --yes html-minifier-terser \
  --collapse-whitespace \
  --conservative-collapse \
  --remove-comments \
  --minify-css true \
  --minify-js false \
  --output "$TMP_HTML" \
  web/index.html >/dev/null

{
  printf '// digitdns-minified:sha256=%s\n' "$JS_HASH"
  cat "$TMP_JS"
} > web/js/app.min.js

{
  printf '/* digitdns-minified:sha256=%s */\n' "$CSS_HASH"
  cat "$TMP_CSS"
} > web/css/style.min.css

{
  printf '<!-- digitdns-minified:sha256=%s -->\n' "$HTML_HASH"
  cat "$TMP_HTML"
} > web/index.min.html

# Brotli sidecars for the assets whose bytes are served unchanged. The Go
# toolchain has no brotli encoder, so the build produces them once; the server
# attaches each to its asset and negotiates it ahead of gzip. index.html is
# excluded: its served body is rewritten per admin namespace at runtime, so a
# sidecar built from the source would not describe the bytes sent.
#
# brotli is required rather than optional: skipping it would leave the previous
# .br files in place after a source edit, and a brotli-capable browser would then
# be served stale JavaScript while a gzip client got the new bytes.
if ! command -v brotli >/dev/null 2>&1; then
  printf 'error: brotli is required to refresh the .br sidecars (install it, then re-run)\n' >&2
  exit 1
fi
for asset in \
  web/js/app.min.js \
  web/css/style.min.css \
  web/css/tailwind.purged.css \
  web/js/i18n.js \
  web/js/feather.min.js \
  web/js/chart.umd.min.js
do
  brotli -q 11 -f -o "$asset.br" "$asset"
done
printf 'wrote brotli sidecars for 6 assets\n'

printf 'wrote web/js/app.min.js (%s), web/css/style.min.css (%s) and web/index.min.html (%s)\n' "$JS_HASH" "$CSS_HASH" "$HTML_HASH"
