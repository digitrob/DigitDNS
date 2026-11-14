const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The Domains table's sinkhole projection. The Custom Blocklist can hold hundreds
// of names, so the plain view collapses the blocked set into one expandable entry
// instead of burying every other rule under a wall of blocked rows.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('function buildDomainEntries'),
  source.indexOf('// sinkholeGroupRow is the one-line entry'),
);

function build(rows, opts) {
  const context = vm.createContext({});
  vm.runInContext(code, context);
  const out = vm.runInContext('buildDomainEntries', context)(rows, opts);
  // The VM has its own Array prototype, so copy into this realm before comparing.
  return {
    head: Array.from(out.head, (e) => ({ ...e })),
    flat: Array.from(out.flat, (e) => ({ ...e })),
  };
}

const rows = [
  { domain: 'proxy.example', status: 'proxy' },
  { domain: 'ads.one', status: 'blocked' },
  { domain: 'ads.two', status: 'blocked' },
  { domain: 'direct.example', status: 'direct' },
];

test('groups the blocked set into one head entry by default', () => {
  const out = build(rows, { groupSinkhole: true, expanded: false, visible: 50 });
  assert.deepEqual(out.head.map((e) => e.kind), ['sinkhole']);
  assert.equal(out.head[0].count, 2);
  assert.deepEqual(out.flat.map((e) => e.domain), ['proxy.example', 'direct.example']);
});

test('expanded shows members and a show-more entry past the window', () => {
  const many = Array.from({ length: 7 }, (_, i) => ({ domain: `b${i}`, status: 'blocked' }));
  const out = build(many, { groupSinkhole: true, expanded: true, visible: 3 });
  assert.deepEqual(out.head.map((e) => e.kind), [
    'sinkhole', 'row', 'row', 'row', 'sinkhole-more',
  ]);
  assert.equal(out.head[4].remaining, 4);
  assert.equal(out.flat.length, 0);
});

test('a status chip or a search keeps the rows flat', () => {
  const out = build(rows, { groupSinkhole: false, expanded: false, visible: 50 });
  assert.equal(out.head.length, 0);
  assert.deepEqual(out.flat.map((e) => e.domain), rows.map((r) => r.domain));
});

test('no blocked rows means no group entry', () => {
  const out = build(
    [{ domain: 'a', status: 'proxy' }],
    { groupSinkhole: true, expanded: false, visible: 50 },
  );
  assert.equal(out.head.length, 0);
  assert.equal(out.flat.length, 1);
});
