const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

// The Domains toolbar is one wrapping flex row. The search field is the only
// item that grows, so any sibling whose width follows its text resizes the
// search field. The selection count used to be min-width:5rem with a ~7rem
// label, which shrank the moment a domain was ticked and moved the field (and
// every bulk button after it). These tests pin the fixed slot that prevents it.
const css = readFileSync(resolve(__dirname, '../../web/css/style.css'), 'utf8');
const html = readFileSync(resolve(__dirname, '../../web/index.html'), 'utf8');

function rule(selector) {
  const start = css.indexOf(selector + ' {');
  assert.ok(start >= 0, `${selector} is missing from style.css`);
  const end = css.indexOf('}', start);
  assert.ok(end > start, `${selector} has no closing brace`);
  return css.slice(start, end);
}

test('the selection count occupies a fixed slot', () => {
  const body = rule('#proxied-selected-count');
  assert.match(body, /flex:\s*0\s+0\s+auto/, 'flex-basis must be fixed, not content-sized');
  const width = body.match(/min-width:\s*([\d.]+)rem/);
  assert.ok(width, 'the count needs an explicit width');
  assert.ok(
    Number(width[1]) >= 7.5,
    `the slot (${width[1]}rem) is narrower than "No domains selected" (~7rem)`,
  );
  assert.match(body, /text-align:\s*end/, 'the short label should sit beside the bulk buttons');
  assert.match(body, /white-space:\s*nowrap/, 'the count must stay on one line');
});

test('the search field keeps a stable grow basis beside the count', () => {
  const body = rule('.proxied-search-box');
  assert.match(body, /flex:\s*1\s+1\s+[\d.]+rem/, 'the search must keep a rem basis and a floor');
  assert.match(body, /min-width:\s*[\d.]+rem/);
});

test('the count is a polite live region', () => {
  assert.match(
    html,
    /id="proxied-selected-count"[^>]*role="status"[^>]*aria-live="polite"/,
    'screen readers should hear the selection count change',
  );
});
