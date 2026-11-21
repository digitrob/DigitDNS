const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The live log's PROTO column colour is a fixed lookup; anything else stays muted.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('const PROTOCOL_ROLES'),
  source.indexOf('// queryRowHTML renders one query'),
);
const context = vm.createContext({});
vm.runInContext(code, context);
const protocolClass = vm.runInContext('protocolClass', context);
const protocolPillClass = vm.runInContext('protocolPillClass', context);

test('each shown protocol has one role for both chip and text', () => {
  const expected = {
    UDP: ['text-cyan-400', 'proto-udp'],
    TCP: ['text-blue-400', 'proto-tcp'],
    DoT: ['text-emerald-400', 'proto-dot'],
    DoH: ['text-purple-400', 'proto-doh'],
    DoQ: ['text-amber-400', 'proto-doq'],
    DoH3: ['text-orange-400', 'proto-doh3'],
    DNSCrypt: ['text-fuchsia-400', 'proto-dnscrypt'],
  };
  for (const [protocol, [text, pill]] of Object.entries(expected)) {
    assert.equal(protocolClass(protocol), text, protocol);
    assert.equal(protocolPillClass(protocol), pill, protocol);
  }
});

test('unknown and blank protocols stay muted in both shapes', () => {
  for (const value of ['', 'SOMETHING', undefined]) {
    assert.equal(protocolClass(value), 'text-slate-500');
    assert.equal(protocolPillClass(value), 'proto-other');
  }
});
