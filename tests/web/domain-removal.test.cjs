const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The Domains table can hold a row in any of four lists, and it hides a *.base
// rule when base sits in the same list. Removal therefore has two jobs the old
// handler did not do: reach the blocked/direct lists, and take the hidden twin
// with it. Lift the pure helpers out of app.js and run them in a bare context.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const helperCode = source.slice(
  source.indexOf('// domainRemovalPatterns is every stored pattern'),
  source.indexOf('// proxiedFilterNeedle is what the table'),
);

function load(rules) {
  const context = vm.createContext({
    currentConfig: { rules },
    DOMAIN_RULE_LISTS: ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct'],
  });
  vm.runInContext(helperCode, context);
  return context;
}

test('a plain row with a same-list wildcard removes both patterns', () => {
  const context = load({ custom_proxied: ['example.com', '*.example.com'] });
  assert.deepEqual(
    [...vm.runInContext('domainRemovalPatterns', context)('example.com')],
    ['example.com', '*.example.com'],
  );
});

test('a wildcard row only removes itself', () => {
  const context = load({ custom_blocked: ['*.example.com', 'example.com'] });
  assert.deepEqual(
    [...vm.runInContext('domainRemovalPatterns', context)('*.example.com')],
    ['*.example.com'],
  );
});

test('a wildcard in another list is a separate row and stays', () => {
  const context = load({ custom_proxied: ['example.com'], custom_blocked: ['*.example.com'] });
  assert.deepEqual(
    [...vm.runInContext('domainRemovalPatterns', context)('example.com')],
    ['example.com'],
  );
});

test('removal reaches blocked and direct rows, not just the proxied pair', () => {
  const context = load({
    custom_proxied: ['keep.test'],
    custom_proxied_disabled: ['paused.test'],
    custom_blocked: ['blocked.test'],
    custom_direct: ['direct.test'],
    custom_domain_added: { 'blocked.test': '2026-10-01T00:00:00Z', 'keep.test': '2026-10-01T00:00:00Z' },
  });
  const removed = vm.runInContext('removeDomainEverywhere', context)('blocked.test');
  assert.equal(removed, true);
  const rules = context.currentConfig.rules;
  assert.deepEqual([...rules.custom_blocked], []);
  assert.deepEqual([...rules.custom_direct], ['direct.test']);
  assert.deepEqual([...rules.custom_proxied], ['keep.test']);
  assert.equal(rules.custom_domain_added['blocked.test'], undefined);
  assert.equal(rules.custom_domain_added['keep.test'], '2026-10-01T00:00:00Z');
});
