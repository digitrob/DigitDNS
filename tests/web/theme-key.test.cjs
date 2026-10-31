const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

// The theme choice is stored under digitdns_theme. The module used to write the
// pre-rename hyperdns_theme key while the pre-paint script read the canonical one
// first, so toggling Light/Dark could silently revert on the next load.
const i18n = readFileSync(resolve(__dirname, '../../web/js/i18n.js'), 'utf8');
const index = readFileSync(resolve(__dirname, '../../web/index.html'), 'utf8');

test('the theme module writes the canonical key', () => {
  assert.match(i18n, /var THEME_KEY = 'digitdns_theme';/);
});

test('the theme module migrates the legacy key instead of keeping it', () => {
  assert.match(i18n, /var LEGACY_THEME_KEY = 'hyperdns_theme';/);
  assert.match(i18n, /localStorage\.removeItem\(LEGACY_THEME_KEY\)/);
});

test('the theme module never writes the legacy key directly', () => {
  assert.doesNotMatch(i18n, /localStorage\.setItem\('hyperdns_theme'/);
});

test('the pre-paint script reads the canonical key first', () => {
  assert.match(index, /localStorage\.getItem\('digitdns_theme'\) \|\| localStorage\.getItem\('hyperdns_theme'\)/);
});
