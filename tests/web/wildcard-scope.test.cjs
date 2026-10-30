const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The quick-action menu's scope logic is pure string work: resolve a domain under
// a scope, and pick the scope a name is already configured with. Lift it out of
// app.js and run it in a bare context, the same way bulk-import.test.cjs does.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const scopeCode = source.slice(
  source.indexOf('// streamManagedPatterns is every domain pattern'),
  source.indexOf('function updateStreamActionScopes'),
);

function load(rules = null) {
  const context = vm.createContext({ currentConfig: rules ? { rules } : null });
  vm.runInContext(scopeCode, context);
  return {
    target: (domain, scope) => vm.runInContext('streamActionTarget', context)(domain, scope),
    targets: (domain, scope) => [...vm.runInContext('streamScopeTargets', context)(domain, scope)],
    scopeFor: (domain, patterns) => vm.runInContext('currentStreamScope', context)(domain, patterns),
    patterns: () => vm.runInContext('streamManagedPatterns', context)(),
  };
}

test('resolves exact, host and parent for a plain host', () => {
  const { target, targets } = load();
  assert.equal(target('s-imp.rmp.rakuten.com', 'exact'), 's-imp.rmp.rakuten.com');
  assert.equal(target('s-imp.rmp.rakuten.com', 'host'), '*.s-imp.rmp.rakuten.com');
  assert.equal(target('s-imp.rmp.rakuten.com', 'parent'), '*.rmp.rakuten.com');
});

test('refuses a parent wildcard that would name a TLD', () => {
  const { target } = load();
  assert.equal(target('example.com', 'parent'), '');
  assert.equal(target('', 'exact'), '');
});

test('a wildcard row rebases instead of disabling its scopes', () => {
  const { target, targets } = load();
  // exact and host are the row itself; *.*.base is never written.
  // Exact drops the wildcard and keeps the host.
  assert.equal(target('*.a.b.c', 'exact'), 'a.b.c');
  assert.equal(target('*.a.b.c', 'host'), '*.a.b.c');
  // parent drops one label from the base.
  assert.equal(target('*.a.b.c', 'parent'), '*.b.c');
  assert.equal(target('*.b.c', 'parent'), '');
  // A bare *.com would cover the whole TLD.
  assert.equal(target('*.com', 'host'), '');
});

test('opens on the scope the lists already hold', () => {
  const { scopeFor } = load();
  assert.equal(scopeFor('a.b.c', new Set(['a.b.c'])), 'exact');
  assert.equal(scopeFor('a.b.c', new Set(['*.a.b.c'])), 'host');
  assert.equal(scopeFor('a.b.c', new Set(['*.b.c'])), 'parent');
  assert.equal(scopeFor('a.b.c', new Set()), 'exact');
  // A plain rule already covers its subdomains, so it wins over a redundant wildcard.
  assert.equal(scopeFor('a.b.c', new Set(['a.b.c', '*.a.b.c'])), 'exact');
});

test('a wildcard row preselects its own host wildcard', () => {
  const { scopeFor } = load();
  assert.equal(scopeFor('*.a.b.c', new Set(['*.a.b.c'])), 'host');
  // Its own rule still wins when a broader parent rule also exists.
  assert.equal(scopeFor('*.a.b.c', new Set(['*.a.b.c', '*.b.c'])), 'host');
  // With no rule of its own, the parent wildcard is the match.
  assert.equal(scopeFor('*.a.b.c', new Set(['*.b.c'])), 'parent');
});

test('collects patterns from all four routing lists', () => {
  const { patterns } = load({
    custom_proxied: ['one.example'],
    custom_proxied_disabled: ['two.example'],
    custom_blocked: ['*.three.example'],
    custom_direct: ['Four.Example.'],
  });
  assert.deepEqual(
    [...patterns()].sort(),
    ['*.three.example', 'four.example', 'one.example', 'two.example'],
  );
  assert.equal(load().patterns().size, 0);
});
