const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const start = source.indexOf('let adGuardSyncInitialized = false;');
const end = source.indexOf('function initFiltering() {');
assert.ok(start > 0 && end > start, 'AdGuard sync block markers are stale');
const code = source.slice(start, end);

function harness(preview) {
  const values = {
    'adguard-sync-source-select': 'src-1',
    'adguard-sync-source-name': 'Norwich',
    'adguard-sync-source-url': 'https://example.test/adguard.txt',
    'adguard-sync-source-text': '[/new.example/]https://host:8443/dns-query\n',
    'adguard-sync-managed-upstream': 'https://host:8443/dns-query',
    'adguard-sync-removal-mode': 'keep',
  };
  const checked = {
    'adguard-sync-enabled': true,
    'adguard-sync-confirm-mirror': false,
    'adguard-sync-ack-risk': false,
  };
  const elements = new Map();
  const get = (id) => {
    if (!elements.has(id)) {
      elements.set(id, {
        value: Object.prototype.hasOwnProperty.call(values, id) ? values[id] : '',
        checked: Object.prototype.hasOwnProperty.call(checked, id) ? checked[id] : false,
        textContent: '',
        innerHTML: '',
        disabled: false,
      });
    }
    return elements.get(id);
  };
  const calls = [];
  const toasts = [];
  let rendered = 0;
  const context = vm.createContext({
    document: { getElementById: get },
    authToken: 'test-token',
    api: (path) => path,
    fetch: async (path, options) => {
      calls.push({ path, body: options.body ? JSON.parse(options.body) : null });
      return {
        ok: true,
        json: async () => (path.endsWith('/preview') ? preview : { success: true, source: { id: 'src-1' }, generated_block: '' }),
      };
    },
    showToast: (message, tone) => toasts.push({ message, tone }),
    errorMessage: async (_res, fallback) => fallback,
    escapeHTML: (value) => String(value),
    confirmAction: async () => true,
    copyClipboard: async () => {},
    currentConfig: { rules: {} },
    renderProxiedList: () => { rendered += 1; },
    console,
  });
  vm.runInContext(code, context);
  return { context, calls, toasts, get, values, checked, rendered: () => rendered };
}

const preview = {
  source_id: 'src-1',
  text: '[/new.example/]https://host:8443/dns-query\n',
  checksum: 'abc123',
  upstream: 'https://host:8443/dns-query',
  warnings: [],
  counts: { covered: 1, adguardOnly: 2, digitdnsOnly: 1, conflict: 0 },
  rows: [
    { domain: 'new.example', bucket: 'adguardOnly', group: 'Streaming', target: 'https://host:8443/dns-query', target_is_self: true },
    { domain: '*.new.test', bucket: 'adguardOnly', group: 'Streaming', target: 'https://host:8443/dns-query', target_is_self: true },
    { domain: 'old.example', bucket: 'covered', group: 'Streaming', covered_by: 'old.example', target_is_self: true },
  ],
  managed: [{ domain: 'manual.example', bucket: 'digitdnsOnly' }],
  generated_block: '',
};

test('preview sends an empty managed selection and defaults imports to checked', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const call = h.calls.find(c => c.path.endsWith('/preview'));
  assert.deepEqual(call.body.managed_selection, [], 'AdGuard export proposal is off by default');
  const importBoxes = h.get('adguard-sync-results').innerHTML.match(/data-adguard-import="[^"]+"[^>]*>/g) || [];
  assert.equal(importBoxes.length, 2);
  assert.equal(importBoxes.filter(box => box.includes(' checked>')).length, 2, 'uncovered domains start checked');
  const managedBoxes = h.get('adguard-sync-results').innerHTML.match(/data-adguard-managed="[^"]+"[^>]*>/g) || [];
  assert.equal(managedBoxes.length, 1);
  assert.equal(managedBoxes[0].includes(' checked>'), false, 'AdGuard export proposal starts unchecked');
});

test('apply carries the exact preview text and checksum into the request', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  vm.runInContext('loadAdGuardSources = async () => null; previewAdGuardSync = async () => null;', h.context);
  await vm.runInContext('applyAdGuardSync()', h.context);
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.ok(call, 'apply request was sent');
  assert.equal(call.body.text, preview.text);
  assert.equal(call.body.checksum, preview.checksum);
  assert.deepEqual(call.body.import, ['new.example', '*.new.test']);
  assert.deepEqual(call.body.managed, []);
  assert.equal(call.body.removal_mode, 'keep');
  assert.equal(call.body.confirm_mirror, false);
  assert.ok(h.calls.some(c => c.path === '/api/config'), 'cached rules are refreshed');
  assert.ok(h.rendered() > 0, 'Domains list is redrawn after apply');
});

test('mirror mode refuses to run without its separate checkbox', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  h.get('adguard-sync-removal-mode').value = 'mirror';
  await vm.runInContext('applyAdGuardSync()', h.context);
  assert.equal(h.calls.some(c => c.path.endsWith('/apply')), false);
  assert.ok(h.toasts.some(t => /Confirm mirror mode/.test(t.message)));
});
