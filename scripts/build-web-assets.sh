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

TMP_JS="$(mktemp)"
TMP_CSS="$(mktemp)"
trap 'rm -f "$TMP_JS" "$TMP_CSS"' EXIT

npx --yes esbuild web/js/app.js --minify --charset=utf8 --target=es2020 --outfile="$TMP_JS" >/dev/null
npx --yes esbuild web/css/style.css --minify --charset=utf8 --target=es2020 --outfile="$TMP_CSS" >/dev/null

{
  printf '// digitdns-minified:sha256=%s\n' "$JS_HASH"
  cat "$TMP_JS"
} > web/js/app.min.js

{
  printf '/* digitdns-minified:sha256=%s */\n' "$CSS_HASH"
  cat "$TMP_CSS"
} > web/css/style.min.css

printf 'wrote web/js/app.min.js (%s) and web/css/style.min.css (%s)\n' "$JS_HASH" "$CSS_HASH"
