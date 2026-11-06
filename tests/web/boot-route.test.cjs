const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

const root = resolve(__dirname, '../..');
const index = readFileSync(resolve(root, 'web/index.html'), 'utf8');
const source = readFileSync(resolve(root, 'web/js/app.js'), 'utf8');
const css = readFileSync(resolve(root, 'web/css/style.css'), 'utf8');

function objectLiteral(text, marker) {
  const start = text.indexOf(marker);
  assert.ok(start >= 0, `${marker} is missing`);
  const open = text.indexOf('{', start);
  const close = text.indexOf('};', open);
  assert.ok(open >= 0 && close > open, `${marker} has no object body`);
  return text.slice(open, close + 1);
}

test('the pre-paint route map matches the dashboard router', () => {
  const boot = objectLiteral(index, 'var routes =');
  const runtime = objectLiteral(source, 'const routeTabs =');
  assert.deepEqual(
    Function(`return (${boot})`)(),
    Function(`return (${runtime})`)(),
  );
});

test('the stylesheet gives boot routing a tab-specific selector', () => {
  for (const tab of ['dashboard', 'proxied', 'filters', 'agents', 'clients', 'stream', 'api', 'rules', 'connect']) {
    assert.ok(
      css.includes(`html[data-boot-tab="${tab}"]`),
      `no boot selector for ${tab}`,
    );
  }
});
