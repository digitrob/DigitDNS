const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// A copy glyph beside the domain name in the Domains table and the Live Log. It
// has to be an inline SVG (live rows are not passed through feather), carry the
// exact stored pattern, and be announceable from the keyboard.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('const DOMAIN_COPY_ICON'),
  source.indexOf('function renderProxiedList'),
);

function load() {
  const context = vm.createContext({
    escapeHTML: (value) => String(value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;'),
  });
  vm.runInContext(code, context);
  return (domain) => vm.runInContext('domainCopyButton', context)(domain);
}

test('the copy button carries the exact pattern, wildcard prefix included', () => {
  const button = load();
  const html = button('*.googleapis.com');
  assert.match(html, /type="button"/);
  assert.match(html, /class="domain-copy-btn"/);
  assert.match(html, /data-copy-value="\*\.googleapis\.com"/);
  assert.match(html, /aria-label="Copy \*\.googleapis\.com"/);
  assert.match(html, /title="Copy domain"/);
  assert.match(html, /<svg /);
});

test('the glyph is escaped like every other stored value', () => {
  const button = load();
  const html = button('a"b.example');
  assert.doesNotMatch(html, /data-copy-value="a"b\.example"/);
  assert.match(html, /data-copy-value="a&quot;b\.example"/);
});

test('both tables render the shared helper', () => {
  // Loose string checks: the template call is what wires the button into the row.
  assert.match(source, /\$\{domainCopyButton\(domain\)\}/);
  assert.match(source, /document\.getElementById\('custom-proxied-rows'\)\?\.addEventListener\('click'/);
  assert.match(source, /const copyBtn = e\.target\.closest\('\[data-copy-value\]'\)/);
});
