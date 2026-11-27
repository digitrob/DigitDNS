const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// DNSCrypt stamps keep the provider host inside the base64 payload, so a raw
// substring check on the sdns:// token never matches. The preset browser must
// still show the provider's bundled mark.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('// DNS stamps are base64 of the binary stamp'),
  source.indexOf('function providerFaviconMarkup'),
);

function providerId(address) {
  const context = vm.createContext({
    atob: (value) => Buffer.from(value, 'base64').toString('binary'),
  });
  vm.runInContext(code, context);
  return vm.runInContext('providerIdForUpstream', context)(address);
}

test('a Quad9 DNSCrypt stamp keeps the Quad9 mark', () => {
  const stamp = 'sdns://AQYAAAAAAAAADTkuOS45LjExOjg0NDMgZ8hHuMh1jNEgJFVDvnVnRt803x2EwAuMRwNo34Idhj4XMi5kbnNjcnlwdC1jZXJ0LnF1YWQ5Lm5ldA';
  assert.equal(providerId(stamp), 'quad9');
});

test('the current Quad9 preset stamp also resolves to Quad9', () => {
  const stamp = 'sdns://AQMAAAAAAAAADDkuOS45Ljk6ODQ0MyBnyEe4yHWM0SAkVUO-dWdG3zTfHYTAC4xHA2jfgh2GPhkyLmRuc2NyeXB0LWNlcnQucXVhZDkubmV0';
  assert.equal(providerId(stamp), 'quad9');
});

test('OpenDNS and AdGuard DNSCrypt stamps map to their own marks', () => {
  const opendns = 'sdns://AQAAAAAAAAAADjIwOC42Ny4yMjAuMjIwILc1EUAgbyJdPivYItf9aR6hwzzI1maNDL4Ev6vKQ_t5GzIuZG5zY3J5cHQtY2VydC5vcGVuZG5zLmNvbQ';
  const adguard = 'sdns://AQIAAAAAAAAAETk0LjE0MC4xNC4xNDo1NDQzINErR_JS3PLCu_iZEIbq95zkSV2LFsigxDIuUso_OQhzIjIuZG5zY3J5cHQuZGVmYXVsdC5uczEuYWRndWFyZC5jb20';
  assert.equal(providerId(opendns), 'opendns');
  assert.equal(providerId(adguard), 'adguard');
});

test('an unknown DNSCrypt stamp stays generic instead of claiming a provider', () => {
  const unknown = 'sdns://AQcAAAAAAAAADTE5Mi4wLjIuMTo1NDQzILc1EUAgbyJdPivYItf9aR6hwzzI1maNDL4Ev6vKQ_t5GzIuZG5zY3J5cHQtY2VydC5leGFtcGxlLmNvbQ';
  assert.equal(providerId(unknown), '');
});
