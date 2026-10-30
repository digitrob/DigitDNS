const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The live log's PROTO column colour is a fixed lookup; anything else stays muted.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('const PROTO_COLORS'),
  source.indexOf('// queryRowHTML renders one query'),
);
const context = vm.createContext({});
vm.runInContext(code, context);
const protocolClass = vm.runInContext('protocolClass', context);

test('each shown protocol has its own colour', () => {
  assert.equal(protocolClass('UDP'), 'text-blue-400');
  assert.equal(protocolClass('TCP'), 'text-sky-400');
  assert.equal(protocolClass('DoT'), 'text-emerald-400');
  assert.equal(protocolClass('DoH'), 'text-purple-400');
  assert.equal(protocolClass('DoQ'), 'text-amber-400');
  assert.equal(protocolClass('DoH3'), 'text-orange-400');
  assert.equal(protocolClass('DNSCrypt'), 'text-fuchsia-400');
});

test('unknown and blank protocols stay muted', () => {
  assert.equal(protocolClass(''), 'text-slate-500');
  assert.equal(protocolClass('SOMETHING'), 'text-slate-500');
  assert.equal(protocolClass(undefined), 'text-slate-500');
});
