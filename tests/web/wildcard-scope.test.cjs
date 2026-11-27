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
    covers: (domain, targets) => vm.runInContext('scopeCoversDomain', context)(domain, targets),
    migrate: (rules, olds, targets, profiles) =>
      vm.runInContext('migrateDomainAttributes', context)(rules, olds, targets, profiles),
    hints: () => vm.runInContext('STREAM_SCOPE_HINTS', context),
    patterns: () => vm.runInContext('streamManagedPatterns', context)(),
    manual: (rules, add, remove) => {
      vm.runInContext('setStreamManualOverrides', context)(rules, add, remove);
      return Array.from(rules.manual_overrides);
    },
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

test('opens on a scope that covers the clicked host', () => {
  const { scopeFor } = load();
  assert.equal(scopeFor('a.b.c', new Set(['a.b.c'])), 'exact');
  assert.equal(scopeFor('a.b.c', new Set(['*.b.c'])), 'parent');
  assert.equal(scopeFor('a.b.c', new Set()), 'exact');
  // A plain rule already covers its subdomains, so it wins over a redundant wildcard.
  assert.equal(scopeFor('a.b.c', new Set(['a.b.c', '*.a.b.c'])), 'exact');
  // A wildcard-only rule leaves the host itself direct. Opening on *.host would
  // write the same wildcard again and the clicked row would not change, so open
  // on the plain rule, which the matcher treats as host + subdomains.
  assert.equal(scopeFor('reflection.ai', new Set(['*.reflection.ai'])), 'exact');
});

test('a scope says whether it still covers the clicked row', () => {
  const { covers } = load();
  assert.equal(covers('reflection.ai', ['reflection.ai']), true);
  // A plain rule covers every subdomain.
  assert.equal(covers('www.reflection.ai', ['reflection.ai']), true);
  // A wildcard rule covers strict subdomains only.
  assert.equal(covers('www.reflection.ai', ['*.reflection.ai']), true);
  assert.equal(covers('reflection.ai', ['*.reflection.ai']), false);
  // A wildcard row stands for its own subtree, so any non-empty scope covers it.
  assert.equal(covers('*.reflection.ai', ['*.reflection.ai']), true);
  assert.equal(covers('reflection.ai', []), false);
});

test('the exact scope names the coverage the matcher actually gives it', () => {
  const { hints } = load();
  // A plain rule is indexed as both exact and wildcard in matcher.go, so "exact"
  // covers subdomains too. The old "this host only" wording was false.
  assert.equal(hints().exact, 'host + subdomains');
});

test('a wildcard row preselects its own host wildcard', () => {
  const { scopeFor } = load();
  assert.equal(scopeFor('*.a.b.c', new Set(['*.a.b.c'])), 'host');
  // Its own rule still wins when a broader parent rule also exists.
  assert.equal(scopeFor('*.a.b.c', new Set(['*.a.b.c', '*.b.c'])), 'host');
  // With no rule of its own, the parent wildcard is the match.
  assert.equal(scopeFor('*.a.b.c', new Set(['*.b.c'])), 'parent');
});

test('a live-log decision records an override marker', () => {
  const { manual } = load({ rules: { manual_overrides: [] } });
  const rules = { manual_overrides: [] };
  assert.deepEqual(manual(rules, ['ads.example'], []), ['ads.example']);
  // A wildcard is stored as one marker, not expanded per host.
  assert.deepEqual(manual(rules, ['*.example.com'], ['ads.example']), ['*.example.com']);
  // Removing the marker returns the domain to ordinary precedence.
  assert.deepEqual(manual(rules, [], ['*.example.com']), []);
});

test('a live-log decision carries its override marker in the save payload', () => {
  assert.match(source, /manual_overrides:\s*currentConfig\.rules\.manual_overrides/);
  assert.match(source, /setStreamManualOverrides\(rules, targets, remove\)/);
});

// A scope change rebases the rule's pattern. The label and relay used to stay on
// the old key, which the server then pruned: the wildcard row came back with no
// service. These pin the move, including the exact key the pickers write.
test('a scope change carries the service and relay to the new key', () => {
  const { migrate } = load();
  const rules = {
    domain_services: { 'a.b.c': 'tv' },
    domain_agents: { 'a.b.c': 'agt_de' },
  };
  const out = migrate(rules, ['a.b.c'], ['*.a.b.c'], {});
  assert.equal(out.key, '*.a.b.c');
  assert.deepEqual({ ...rules.domain_services }, { '*.a.b.c': 'tv' });
  assert.deepEqual({ ...rules.domain_agents }, { '*.a.b.c': 'agt_de' });
});

test('host-plus-suffix targets use the exact host as the attribute key', () => {
  const { migrate } = load();
  const rules = { domain_services: { '*.a.b.c': 'tv' } };
  const out = migrate(rules, ['*.a.b.c'], ['a.b.c', '*.a.b.c'], {});
  assert.equal(out.key, 'a.b.c');
  assert.deepEqual({ ...rules.domain_services }, { 'a.b.c': 'tv' });
});

test('a label already on the target wins and the stale one is dropped', () => {
  const { migrate } = load();
  const rules = {
    domain_services: { 'a.b.c': 'tv', '*.a.b.c': 'radio' },
    domain_agents: { 'a.b.c': 'old', '*.a.b.c': 'new' },
  };
  migrate(rules, ['a.b.c'], ['*.a.b.c'], {});
  assert.deepEqual({ ...rules.domain_services }, { '*.a.b.c': 'radio' });
  assert.deepEqual({ ...rules.domain_agents }, { '*.a.b.c': 'new' });
});

test('an unchanged key is a no-op', () => {
  const { migrate } = load();
  const rules = { domain_services: { 'a.b.c': 'tv' } };
  const out = migrate(rules, ['a.b.c', '*.a.b.c'], ['a.b.c'], {});
  assert.equal(out.key, 'a.b.c');
  assert.deepEqual({ ...rules.domain_services }, { 'a.b.c': 'tv' });
});

test('a resolution profile is reported for the caller to move after the save', () => {
  const { migrate } = load();
  const profiles = { 'a.b.c': { upstream: '1.1.1.1' } };
  const out = migrate({}, ['a.b.c'], ['*.b.c'], profiles);
  assert.equal(out.resolutionFrom, 'a.b.c');
  // The helper never edits the profile map: the rules save has to succeed first.
  assert.deepEqual(profiles, { 'a.b.c': { upstream: '1.1.1.1' } });
});

test('moving a pattern does not steal a separate wildcard row\'s label', () => {
  const { migrate } = load();
  // The plain row has no label; the wildcard row does. A parent-scope change on
  // the plain row must not carry the wildcard row's label along.
  const rules = { domain_services: { '*.a.b.c': 'radio' } };
  migrate(rules, ['a.b.c'], ['*.b.c'], {});
  assert.deepEqual({ ...rules.domain_services }, { '*.a.b.c': 'radio' });
});

test('migrates Added stamp to the new key when target has none', () => {
  const { migrate } = load();
  const rules = {
    custom_domain_added: { 'a.b.c': '2026-01-01T00:00:00.000Z' },
    domain_services: { 'a.b.c': 'tv' },
  };
  const out = migrate(rules, ['a.b.c'], ['*.a.b.c'], {});
  assert.equal(out.key, '*.a.b.c');
  assert.equal(rules.custom_domain_added['*.a.b.c'], '2026-01-01T00:00:00.000Z');
  assert.equal(rules.custom_domain_added['a.b.c'], undefined);
});

test('keeps target Added stamp when already present', () => {
  const { migrate } = load();
  const rules = {
    custom_domain_added: { 'a.b.c': '2026-01-01T00:00:00.000Z', '*.a.b.c': '2026-02-01T00:00:00.000Z' },
  };
  migrate(rules, ['a.b.c'], ['*.a.b.c'], {});
  assert.equal(rules.custom_domain_added['*.a.b.c'], '2026-02-01T00:00:00.000Z');
  assert.equal(rules.custom_domain_added['a.b.c'], undefined);
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
