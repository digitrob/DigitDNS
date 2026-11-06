const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The proxy leak guard answers HTTPS/SVCB locally, so the live log carries a
// PROXY_HTTPS_SINK action rather than PROXY. The filter and the relay cell must
// still treat it as a proxied query, otherwise filtering by PROXY hides exactly
// the queries the guard produced.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const start = source.indexOf('// The live log and the aggregate counters treat');
const end = source.indexOf('// Every action string the resolver can log');
if (start < 0 || end < 0) throw new Error('proxy filter helper block not found');

const context = vm.createContext({});
vm.runInContext(source.slice(start, end), context);
const isProxyAction = vm.runInContext('isProxyAction', context);
const queryMatchesStream = vm.runInContext('queryMatchesStream', context);

test('both sink actions count as proxy actions', () => {
  assert.equal(isProxyAction('PROXY'), true);
  assert.equal(isProxyAction('PROXY_IPV6_SINK'), true);
  assert.equal(isProxyAction('PROXY_HTTPS_SINK'), true);
  assert.equal(isProxyAction('DIRECT'), false);
  assert.equal(isProxyAction('BLOCK'), false);
});

test('the PROXY filter keeps HTTPS and IPv6 sink rows', () => {
  const filter = { action: 'PROXY', search: '' };
  assert.equal(queryMatchesStream({ action: 'PROXY', domain: 'a.test' }, filter), true);
  assert.equal(queryMatchesStream({ action: 'PROXY_HTTPS_SINK', domain: 'a.test' }, filter), true);
  assert.equal(queryMatchesStream({ action: 'PROXY_IPV6_SINK', domain: 'a.test' }, filter), true);
  assert.equal(queryMatchesStream({ action: 'DIRECT', domain: 'a.test' }, filter), false);
});
