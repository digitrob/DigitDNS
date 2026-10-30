const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The Domains row's wildcard chips: given every stored pattern, name the wildcards
// that apply to one row. A wildcard is relative to the row (*.example.com is a host
// wildcard of example.com and a parent wildcard of api.example.com).
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('// wildcardTypesFor names'),
  source.indexOf('function renderProxiedList'),
);

const STATUS_META = {
  proxy: { label: 'PROXIED' },
  paused: { label: 'PAUSED' },
  blocked: { label: 'BLOCKED' },
  direct: { label: 'DIRECT' },
};

function types(domain, entries) {
  const context = vm.createContext({});
  vm.runInContext(code, context);
  const index = new Map(Object.entries(entries));
  return [...vm.runInContext('wildcardTypesFor', context)(domain, index)].map((t) => ({
    type: t.type,
    pattern: t.pattern,
    status: t.status,
  }));
}

function label(domain, entries) {
  const context = vm.createContext({ DOMAIN_STATUS_META: STATUS_META });
  vm.runInContext(code, context);
  const index = new Map(Object.entries(entries));
  return vm.runInContext('wildcardLabelFor', context)(domain, index);
}

test('names the host wildcard of a domain', () => {
  assert.deepEqual(types('a.b.c', { '*.a.b.c': 'blocked' }), [
    { type: '*.host', pattern: '*.a.b.c', status: 'blocked' },
  ]);
});

test('names the parent wildcard of a subdomain', () => {
  assert.deepEqual(types('a.b.c', { '*.b.c': 'proxy' }), [
    { type: '*.parent', pattern: '*.b.c', status: 'proxy' },
  ]);
});

test('names the host and parent wildcard together', () => {
  assert.deepEqual(types('a.b.c', { '*.a.b.c': 'blocked', '*.b.c': 'proxy' }), [
    { type: '*.host', pattern: '*.a.b.c', status: 'blocked' },
    { type: '*.parent', pattern: '*.b.c', status: 'proxy' },
  ]);
});

test('a host wildcard beside a plain rule in the same list is redundant', () => {
  // The plain rule covers the host and every subdomain; the wildcard adds nothing.
  assert.deepEqual(types('a.b.c', { 'a.b.c': 'blocked', '*.a.b.c': 'blocked' }), []);
  // In different lists it still changes what subdomains do.
  assert.deepEqual(types('a.b.c', { 'a.b.c': 'proxy', '*.a.b.c': 'blocked' }), [
    { type: '*.host', pattern: '*.a.b.c', status: 'blocked' },
  ]);
});

test('a wildcard row reports its own host type', () => {
  assert.deepEqual(types('*.a.b.c', { '*.a.b.c': 'blocked' }), [
    { type: '*.host', pattern: '*.a.b.c', status: 'blocked' },
  ]);
});

test('a two-label domain has no parent wildcard', () => {
  assert.deepEqual(types('example.com', { '*.com': 'blocked' }), []);
  assert.deepEqual(types('example.com', {}), []);
});

test('the subtitle says exact when no wildcard applies', () => {
  // The object comes from the VM realm, so copy it into a host-realm object first.
  assert.deepEqual({ ...label('a.b.c', {}) }, {
    text: 'exact',
    title: 'No wildcard rule for this name',
    wildcard: false,
  });
});

test('the subtitle names the active wildcard type and keeps the pattern in its title', () => {
  const row = label('a.b.c', { '*.a.b.c': 'blocked' });
  assert.equal(row.text, '*.host');
  assert.equal(row.title, '*.a.b.c — blocked');
  assert.equal(row.wildcard, true);
});

test('two wildcards become one subtitle line', () => {
  const row = label('a.b.c', { '*.a.b.c': 'blocked', '*.b.c': 'proxy' });
  assert.equal(row.text, '*.host · *.parent');
  assert.equal(row.title, '*.a.b.c — blocked · *.b.c — proxied');
});
