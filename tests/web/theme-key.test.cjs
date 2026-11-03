const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

// The theme choice is stored under digitdns_theme. The module used to write the
// pre-rename hyperdns_theme key while the pre-paint script read the canonical one
// first, so toggling Light/Dark could silently revert on the next load. The legacy
// migration has since been retired; every install that loaded the panel after
// 3.6.1 already carries the canonical key.
const i18n = readFileSync(resolve(__dirname, '../../web/js/i18n.js'), 'utf8');
const index = readFileSync(resolve(__dirname, '../../web/index.html'), 'utf8');

test('the theme module writes the canonical key', () => {
  assert.match(i18n, /var THEME_KEY = 'digitdns_theme';/);
});

test('the theme module no longer reads or writes the retired key', () => {
  assert.doesNotMatch(i18n, /hyperdns_theme/);
});

test('the pre-paint script reads the canonical key', () => {
  assert.match(index, /var t = localStorage\.getItem\('digitdns_theme'\);/);
  assert.doesNotMatch(index, /hyperdns_theme/);
});
