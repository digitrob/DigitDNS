const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The Domains table sorts on custom_domain_added. A row with no stamp sorts as
// epoch 0 and therefore sinks below every stamped row, which is why an Add used
// to need a page reload: the server stamped it on save, but the open page never
// saw that response. These tests pin the client-side stamp and the sort it feeds.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('function proxiedOrderedRows'),
  source.indexOf('// domainRemovalPatterns is every stored pattern'),
);

function load(rules) {
  const context = vm.createContext({
    currentConfig: { rules },
    proxiedSort: 'added-desc',
    DOMAIN_STATUS_PRECEDENCE: { paused: 0, proxy: 1, blocked: 2, direct: 3 },
  });
  vm.runInContext(code, context);
  return context;
}

function ordered(rules) {
  const context = load(rules);
  return Array.from(vm.runInContext('proxiedOrderedRows', context)(), (row) => row.domain);
}

test('a newly added domain sorts above older rows without a reload', () => {
  const rules = {
    custom_proxied: ['old.example', 'new.example'],
    custom_proxied_disabled: [],
    custom_blocked: [],
    custom_direct: [],
    custom_domain_added: { 'old.example': '2026-10-01T00:00:00Z' },
  };
  const context = load(rules);
  vm.runInContext('stampDomainsAsAdded', context)(
    rules,
    ['new.example'],
    '',
    '2026-11-17T12:00:00.123Z',
  );

  assert.equal(rules.custom_domain_added['new.example'], '2026-11-17T12:00:00.123Z');
  assert.deepEqual(ordered(rules), ['new.example', 'old.example']);
});

test('an existing Added stamp is not overwritten', () => {
  const rules = {
    custom_proxied: ['keep.example'],
    custom_proxied_disabled: [],
    custom_blocked: [],
    custom_direct: [],
    custom_domain_added: { 'keep.example': '2026-10-01T00:00:00Z' },
  };
  const context = load(rules);
  vm.runInContext('stampDomainsAsAdded', context)(
    rules,
    ['keep.example'],
    '',
    '2026-11-17T12:00:00.123Z',
  );

  assert.equal(rules.custom_domain_added['keep.example'], '2026-10-01T00:00:00Z');
});

test('a move inherits the stamp left on its wildcard alias', () => {
  const rules = {
    custom_proxied: ['old.example'],
    custom_proxied_disabled: [],
    custom_blocked: [],
    custom_direct: [],
    custom_domain_added: { '*.old.example': '2026-10-01T00:00:00Z' },
  };
  const context = load(rules);
  vm.runInContext('stampDomainsAsAdded', context)(
    rules,
    ['old.example'],
    '',
    '2026-11-17T12:00:00.123Z',
  );

  assert.equal(rules.custom_domain_added['old.example'], '2026-10-01T00:00:00Z');
});

test('the Added map is created when the config predates it', () => {
  const rules = {
    custom_proxied: ['new.example'],
    custom_proxied_disabled: [],
    custom_blocked: [],
    custom_direct: [],
  };
  const context = load(rules);
  vm.runInContext('stampDomainsAsAdded', context)(
    rules,
    ['new.example'],
    '',
    '2026-11-17T12:00:00.123Z',
  );

  assert.equal(rules.custom_domain_added['new.example'], '2026-11-17T12:00:00.123Z');
});
