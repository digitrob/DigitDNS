const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// Adding a domain that an existing plain rule or wildcard already covers creates
// a second rule that can never win. These helpers mirror the server's matcher
// (internal/core/matcher) and drive the Add box's inline note and bulk report.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('function ruleCoversDomain'),
  source.indexOf('// RECORD_DOMAIN_RE'),
);

const LIST_KEYS = ['custom_proxied', 'custom_proxied_disabled', 'custom_blocked', 'custom_direct'];

function load() {
  const context = vm.createContext({ DOMAIN_RULE_LISTS: LIST_KEYS });
  vm.runInContext(code, context);
  return {
    covers: (rule, domain) => vm.runInContext('ruleCoversDomain', context)(rule, domain),
    covering: (domain, list) => vm.runInContext('coveringRuleFor', context)(domain, list),
    info: (domain, rules) => vm.runInContext('coveringRuleInfo', context)(domain, rules),
    subsumed: (rule, lists) => vm.runInContext('subsumedRules', context)(rule, lists),
  };
}

test('a plain rule covers itself and every subdomain', () => {
  const { covers } = load();
  assert.equal(covers('googleapis.com', 'googleapis.com'), true);
  assert.equal(covers('googleapis.com', 'frontend.googleapis.com'), true);
  assert.equal(covers('googleapis.com', 'deep.frontend.googleapis.com'), true);
  // Suffix matching is label-aware: notexample.com is a different name.
  assert.equal(covers('example.com', 'notexample.com'), false);
});

test('a wildcard covers strict subdomains and not its apex', () => {
  const { covers } = load();
  assert.equal(covers('*.googleapis.com', 'frontend.googleapis.com'), true);
  assert.equal(covers('*.googleapis.com', 'googleapis.com'), false);
  assert.equal(covers('*.a.googleapis.com', 'b.a.googleapis.com'), true);
});

test('the most specific covering rule wins', () => {
  const { covering } = load();
  assert.equal(
    covering('api.example.com', ['example.com', 'api.example.com', '*.example.com']),
    'api.example.com',
  );
  assert.equal(covering('api.example.com', ['example.com']), 'example.com');
  assert.equal(covering('example.com', ['*.example.com']), '');
});

test('coverage is reported across all four routing lists', () => {
  const { info } = load();
  const rules = {
    custom_proxied: ['googleapis.com'],
    custom_blocked: ['ads.example'],
    custom_direct: ['*.lan.example'],
  };
  assert.deepEqual(
    { ...info('frontend.googleapis.com', rules) },
    { pattern: 'googleapis.com', status: 'proxy', key: 'custom_proxied' },
  );
  assert.equal(info('cdn.ads.example', rules).status, 'blocked');
  assert.equal(info('host.lan.example', rules).status, 'direct');
  assert.equal(info('unrelated.example', rules), null);
});

test('an exact entry counts as coverage too', () => {
  const { info } = load();
  const rules = { custom_proxied_disabled: ['paused.example'] };
  assert.deepEqual(
    { ...info('paused.example', rules) },
    { pattern: 'paused.example', status: 'paused', key: 'custom_proxied_disabled' },
  );
});

test('a new plain rule names the narrower rules it subsumes', () => {
  const { subsumed } = load();
  const proxy = ['*.googleapis.com', 'api.googleapis.com', 'other.example'];
  const find = (rule) => Array.from(subsumed(rule, [proxy]));
  assert.deepEqual(find('googleapis.com'), ['*.googleapis.com', 'api.googleapis.com']);
  // A wildcard never subsumes: the apex of each child rule may still need it.
  assert.deepEqual(find('*.googleapis.com'), []);
  // The exact same pattern is not "redundant"; it is the rule being added.
  assert.deepEqual(find('other.example'), []);
});
