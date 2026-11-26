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

// The two select checkboxes. The header tick owns the view minus the folded
// Custom Blocklist; the group tick owns the blocklist alone. Before this, the
// header tick selected the hidden blocked rows too, so "select all" quietly
// captured the whole sinkhole and a bulk label hit it.
const selectionCode = source.slice(
  source.indexOf('function proxiedScopedRows'),
  source.indexOf('// syncProxiedSelectionBoxes'),
);

function selection(rows, opts = {}) {
  const context = vm.createContext({
    proxiedStatusFilter: opts.status || null,
    activeService: opts.service || '',
    proxiedFilterNeedle: () => opts.needle || '',
    proxiedFilteredRows: () => (opts.status ? rows.filter((row) => row.status === opts.status) : rows),
    serviceOfDomain: (domain) => (opts.services || {})[domain] || '',
  });
  vm.runInContext(selectionCode, context);
  const pick = (name) => Array.from(
    vm.runInContext(name, context)().map((row) => row.domain),
  );
  return {
    selectAll: pick('proxiedSelectAllRows'),
    sinkhole: pick('proxiedSinkholeRows'),
    grouped: vm.runInContext('proxiedSinkholeGrouped', context)(),
  };
}

const selectionRows = [
  { domain: 'proxy.example', status: 'proxy' },
  { domain: 'ads.one', status: 'blocked' },
  { domain: 'ads.two', status: 'blocked' },
  { domain: 'direct.example', status: 'direct' },
];

test('the header tick excludes the folded Custom Blocklist', () => {
  const out = selection(selectionRows);
  assert.equal(out.grouped, true);
  assert.deepEqual(out.selectAll, ['proxy.example', 'direct.example']);
  assert.deepEqual(out.sinkhole, ['ads.one', 'ads.two']);
});

test('a search flattens the view, so the header tick includes matches', () => {
  const out = selection(selectionRows, { needle: 'ads' });
  assert.equal(out.grouped, false);
  assert.deepEqual(out.selectAll, selectionRows.map((r) => r.domain));
  assert.deepEqual(out.sinkhole, []);
});

test('a status chip flattens the view too', () => {
  const out = selection(selectionRows, { status: 'blocked' });
  assert.equal(out.grouped, false);
  assert.deepEqual(out.selectAll, ['ads.one', 'ads.two']);
  assert.deepEqual(out.sinkhole, []);
});

test('a service filter narrows both select scopes', () => {
  const out = selection(selectionRows, {
    service: 'tv',
    services: { 'proxy.example': 'tv', 'ads.one': 'tv' },
  });
  assert.deepEqual(out.selectAll, ['proxy.example']);
  assert.deepEqual(out.sinkhole, ['ads.one']);
});
