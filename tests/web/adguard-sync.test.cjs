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
    'adguard-sync-source-text': '[/new.example/]https://host:8443/dns-query\n',
    'adguard-sync-removal-mode': 'keep',
  };
  const checked = {
    'adguard-sync-confirm-mirror': false,
    'adguard-sync-hide-covered': false,
  };
  const elements = new Map();
  const get = (id) => {
    if (!elements.has(id)) {
      const attrs = {
        'aria-pressed': 'false',
      };
      elements.set(id, {
        value: Object.prototype.hasOwnProperty.call(values, id) ? values[id] : '',
        checked: Object.prototype.hasOwnProperty.call(checked, id) ? checked[id] : false,
        textContent: '',
        innerHTML: '',
        disabled: false,
        hidden: true,
        classList: { toggle() {}, add() {}, remove() {} },
        setAttribute(name, value) { attrs[name] = String(value); },
        getAttribute(name) { return Object.prototype.hasOwnProperty.call(attrs, name) ? attrs[name] : null; },
        querySelector: () => null,
        closest: () => null,
      });
    }
    return elements.get(id);
  };
  const calls = [];
  const toasts = [];
  const confirms = [];
  let rendered = 0;
  const context = vm.createContext({
    document: { getElementById: get },
    authToken: 'test-token',
    api: (path) => path,
    streamActionMode: 'adguard',
    streamScopeTargets: (domain, scope) => [domain, scope].filter(Boolean),
    providerFaviconMarkup: (address, extraClass = '') =>
      `<img class="resolver-favicon ${extraClass}" src="/img/providers/${address}.png" alt="">`,
    fetch: async (path, options) => {
      calls.push({ path, body: options.body ? JSON.parse(options.body) : null });
      return {
        ok: true,
        json: async () => (path.endsWith('/preview') ? preview : { success: true, source: { id: 'src-1' } }),
      };
    },
    showToast: (message, tone) => toasts.push({ message, tone }),
    errorMessage: async (_res, fallback) => fallback,
    escapeHTML: (value) => String(value),
    confirmAction: async (options) => { confirms.push(options); return true; },
    copyClipboard: async () => {},
    currentConfig: { rules: {} },
    renderProxiedList: () => { rendered += 1; },
    console,
  });
  vm.runInContext(code, context);
  return { context, calls, toasts, confirms, get, values, checked, rendered: () => rendered };
}

const preview = {
  source_id: 'src-1',
  text: '[/new.example/]https://host:8443/dns-query\n',
  checksum: 'abc123',
  upstream: 'https://host:8443/dns-query',
  warnings: [],
  counts: { covered: 1, adguardOnly: 2, digitdnsOnly: 1, conflict: 1 },
  rows: [
    { domain: 'new.example', bucket: 'adguardOnly', group: 'Streaming', target: 'https://host:8443/dns-query', target_is_self: true, in_digitdns: false, suggested_action: 'proxy' },
    { domain: '*.new.test', bucket: 'adguardOnly', group: 'Streaming', target: 'https://host:8443/dns-query', target_is_self: true, in_digitdns: false, suggested_action: 'block', variants: ['new.test'], broad: true },
    { domain: 'old.example', bucket: 'covered', group: 'Streaming', covered_by: 'old.example', target_is_self: true, in_digitdns: true, suggested_action: 'proxy' },
    { domain: 'manual.example', bucket: 'digitdnsOnly', target_is_self: true, in_digitdns: true },
    { domain: 'conflict.example', bucket: 'conflict', covered_by: 'conflict.example', target_is_self: true, in_digitdns: true, suggested_action: 'block' },
  ],
  managed: [{ domain: 'manual.example', bucket: 'digitdnsOnly' }],
  generated_block: '',
};

test('preview only carries import and review fields; imports default to checked', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const call = h.calls.find(c => c.path.endsWith('/preview'));
  assert.deepEqual(call.body.domain_actions, {}, 'no route changes staged yet');
  assert.equal(Object.hasOwn(call.body, 'adblock_domains'), false);
  assert.equal(Object.hasOwn(call.body, 'managed_selection'), false);
  assert.equal(Object.hasOwn(call.body, 'url'), false, 'no source URL is sent by the dashboard');
  assert.equal(Object.hasOwn(call.body, 'enabled'), false, 'saved lists do not carry an enabled flag');
  assert.equal(call.body.removal_mode, 'keep');
  assert.equal(call.body.confirm_mirror, false);
  assert.equal(call.body.acknowledge_risk, true);
  assert.equal(h.calls.some(c => c.path.includes('upstreams')), false, 'no upstream catalogue is loaded');
  const importBoxes = h.get('adguard-sync-results').innerHTML.match(/data-adguard-import="[^"]+"[^>]*>/g) || [];
  assert.equal(importBoxes.length, 2);
  assert.equal(importBoxes.filter(box => box.includes(' checked>')).length, 2, 'uncovered domains start checked');
});

test('review table uses domains-list route chips and row highlights, no status column or adblock', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const html = h.get('adguard-sync-results').innerHTML;
  assert.match(html, /<table class="proxied-table w-full text-start border-collapse text-xs font-mono">/);
  assert.match(html, /<thead class="bg-slate-950\/80 sticky top-0 text-slate-400 border-b border-slate-800 z-10">/);
  assert.match(html, /class="ag-col-route py-2\.5 px-3 text-center"/);
  assert.match(html, /class="proxied-col-domain py-2\.5 px-3 text-start">DOMAIN<\/th>/);
  assert.match(html, /class="ag-col-upstream py-2\.5 px-3 text-start">GROUP\/UPSTREAM<\/th>/);
  assert.match(html, /class="ag-col-route py-2\.5 px-3 text-center">ROUTE<\/th>/);
  assert.doesNotMatch(html, /ag-col-status|>STATUS<|class="ag-status-stack"/, 'the per-row status column is gone');
  assert.doesNotMatch(html, /Group \/ upstream|Group\/Upstream|>Domain<|>Status<|>Route</);
  assert.doesNotMatch(html, /<table class="ag-table/);
  assert.match(html, /class="ag-row-covered[^"]*" data-adguard-state="covered"/);
  assert.match(html, /class="ag-row-conflict[^"]*" data-adguard-state="conflict"/);
  assert.doesNotMatch(html, /ag-pill-covered[^>]*>Already in DigitDNS|ag-pill-add[^>]*>Add to DigitDNS/, 'no per-row status pills');
  assert.match(html, /<span class="sr-only">Already in DigitDNS: <\/span>old\.example/, 'state stays available to assistive tech');
  assert.match(html, /data-adguard-route-domain="new\.example"/);
  assert.match(html, /class="proxied-route-btn adguard-route-btn badge badge-proxy inline-flex items-center gap-1"/);
  const page = readFileSync(resolve(__dirname, '../../web/index.html'), 'utf8');
  for (const label of ['Proxy', 'Block', 'Direct']) {
    assert.match(page, new RegExp(`class="stream-action-item"[\\s\\S]*?<span>${label}</span>`), `${label} action exists in the shared menu`);
  }
  assert.doesNotMatch(html, /data-adguard-status-menu|ag-status-options|data-status=/, 'no importer-only dropdown remains');
  assert.doesNotMatch(html, /data-adguard-adblock|data-adguard-managed/);
  assert.doesNotMatch(html, /Send back to AdGuard|Every proxied domain is already named/);
  assert.doesNotMatch(html, /manual\.example/, 'DigitDNS-only domains are not part of this review');
  assert.doesNotMatch(html, />Note</);
  assert.doesNotMatch(html, /ag-row-meta|not yet in DigitDNS|suggested:|variants:/);
});

test('with no chip pressed the whole parsed list is shown', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const html = h.get('adguard-sync-results').innerHTML;
  assert.match(html, /new\.example/, 'missing domains are visible');
  assert.match(html, /\*\.new\.test/, 'listed wildcards are visible');
  assert.match(html, /old\.example/, 'covered rows are visible');
  assert.match(html, /class="ag-row-covered[^"]*" data-adguard-state="covered"/, 'covered rows are tinted, not chipped');
  assert.match(html, /conflict\.example/, 'conflict rows stay visible');
  assert.doesNotMatch(html, /manual\.example/, 'DigitDNS-only rows stay out of this review');
  assert.doesNotMatch(html, /ag-pill-broad|>Broad</, 'broad rows no longer get a separate chip');
  assert.equal(vm.runInContext('adGuardSyncSelectedOnly', h.context), false);
  assert.equal(vm.runInContext('adGuardSyncAlreadyOnly', h.context), false);
});

test('a loaded preview status is green, not the pending purple chip', () => {
  const h = harness(preview);
  vm.runInContext("setAdGuardSyncStatus('Preview ready', 'ready')", h.context);
  assert.equal(h.get('adguard-sync-status').className, 'ag-pill ag-pill-covered');
});

test('Already in DigitDNS chip narrows the table to covered rows', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  await vm.runInContext('adGuardSyncSetFilterChips(false, true)', h.context);
  const html = h.get('adguard-sync-results').innerHTML;
  assert.match(html, /old\.example/);
  assert.doesNotMatch(html, /new\.example|\*\.new\.test|conflict\.example|manual\.example/);
  assert.equal(h.get('adguard-sync-hide-covered').getAttribute('aria-pressed'), 'true');
});

test('Selected Only chip narrows the table to checked rows and replaces the other chip', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  await vm.runInContext('adGuardSyncSetFilterChips(false, true)', h.context);
  await vm.runInContext('adGuardSyncSetFilterChips(true, false)', h.context);
  assert.equal(h.get('adguard-sync-selected-only').getAttribute('aria-pressed'), 'true');
  assert.equal(h.get('adguard-sync-hide-covered').getAttribute('aria-pressed'), 'false');
  const html = h.get('adguard-sync-results').innerHTML;
  assert.match(html, /new\.example/);
  assert.match(html, /\*\.new\.test/);
  assert.doesNotMatch(html, /conflict\.example|old\.example|manual\.example/);
});

test('clearing the active chip restores the whole parsed list', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  for (const activate of ['adGuardSyncSetFilterChips(false, true)', 'adGuardSyncSetFilterChips(true, false)']) {
    await vm.runInContext(activate, h.context);
    await vm.runInContext('adGuardSyncSetFilterChips(false, false)', h.context);
    const html = h.get('adguard-sync-results').innerHTML;
    assert.match(html, /new\.example/);
    assert.match(html, /old\.example/);
    assert.match(html, /conflict\.example/);
    assert.equal(h.get('adguard-sync-selected-only').getAttribute('aria-pressed'), 'false');
    assert.equal(h.get('adguard-sync-hide-covered').getAttribute('aria-pressed'), 'false');
  }
});

test('apply carries the exact preview text and checksum with import routes only', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  await vm.runInContext('applyAdGuardSync()', h.context);
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.ok(call, 'apply request was sent');
  assert.equal(call.body.text, preview.text);
  assert.equal(call.body.checksum, preview.checksum);
  assert.deepEqual(call.body.import, ['new.example', '*.new.test']);
  assert.deepEqual(call.body.domain_actions, {});
  assert.equal(Object.hasOwn(call.body, 'managed'), false);
  assert.equal(Object.hasOwn(call.body, 'managed_selection'), false);
  assert.equal(Object.hasOwn(call.body, 'adblock_domains'), false);
  assert.equal(call.body.removal_mode, 'keep');
  assert.ok(h.calls.some(c => c.path === '/api/config'), 'cached rules are refreshed');
  assert.ok(h.rendered() > 0, 'Domains list is redrawn after apply');
});

test('staged routes refresh the preview and ride in the apply payload', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  vm.runInContext(`adGuardSyncSetRoute('new.example', 'block')`, h.context);
  assert.equal(vm.runInContext('adGuardSyncReviewDirty', h.context), true);
  assert.match(h.get('adguard-sync-results').innerHTML, /data-adguard-route-domain="new\.example"[^>]*>Blocked</, 'route chip refreshes in place');
  await vm.runInContext('applyAdGuardSync()', h.context);
  const previews = h.calls.filter(c => c.path.endsWith('/preview'));
  // One initial preview, one checksum refresh before apply, and one reset
  // preview after a successful apply.
  assert.equal(previews.length, 3, 'a stale checksum forces one fresh preview');
  assert.deepEqual(previews[1].body.domain_actions, { 'new.example': 'block' });
  assert.equal(Object.hasOwn(previews[1].body, 'adblock_domains'), false);
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.equal(call.body.checksum, preview.checksum);
  assert.deepEqual(call.body.domain_actions, { 'new.example': 'block' });
  assert.equal(vm.runInContext('adGuardSyncReviewDirty', h.context), false);
});

test('wildcard scope stages every Domains-list pattern in the apply payload', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  vm.runInContext(`adGuardSyncSetRoute('new.example', 'block', ['*.new.example'])`, h.context);
  const fromVM = (expression) => JSON.parse(vm.runInContext(`JSON.stringify(${expression})`, h.context));
  assert.deepEqual(fromVM('adGuardSyncImportList()'), ['*.new.example', '*.new.test'], 'the scope keeps its origin slot like the Domains list');
  assert.deepEqual(fromVM('Array.from(adGuardSyncDomainActions.entries())'), [
    ['*.new.example', 'block'],
  ]);
  await vm.runInContext('applyAdGuardSync()', h.context);
  const refresh = h.calls.filter(c => c.path.endsWith('/preview'))[1];
  assert.deepEqual(refresh.body.domain_actions, {
    '*.new.example': 'block',
  });
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.deepEqual(call.body.import, ['*.new.example', '*.new.test']);
  assert.deepEqual(call.body.domain_actions, {
    '*.new.example': 'block',
  });

  // Switching back to exact drops the old wildcard instead of leaving it staged.
  vm.runInContext(`adGuardSyncSetRoute('new.example', 'block', ['new.example'])`, h.context);
  assert.deepEqual(fromVM('adGuardSyncImportList()'), ['new.example', '*.new.test']);
  assert.deepEqual(fromVM('Array.from(adGuardSyncDomainActions.entries())'), [
    ['new.example', 'block'],
  ]);
});

test('a wildcard from AdGuard is kept as listed, never widened into another wildcard', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const fromVM = (expression) => JSON.parse(vm.runInContext(`JSON.stringify(${expression})`, h.context));
  assert.deepEqual(fromVM(`adGuardSyncScopeTargets('*.new.test', 'exact')`), ['*.new.test']);
  assert.deepEqual(fromVM(`adGuardSyncScopeTargets('*.new.test', 'host')`), []);
  assert.deepEqual(fromVM(`adGuardSyncScopeTargets('*.new.test', 'parent')`), []);

  vm.runInContext(`adGuardSyncSelectedImport = new Set(['*.new.test'])`, h.context);
  vm.runInContext(`adGuardSyncSetRoute('*.new.test', 'block', adGuardSyncScopeTargets('*.new.test', 'exact'))`, h.context);
  assert.deepEqual(fromVM('adGuardSyncImportList()'), ['*.new.test']);
  assert.deepEqual(fromVM('Array.from(adGuardSyncDomainActions.entries())'), [['*.new.test', 'block']]);
});

test('already-present rows keep a route chip and explicit edits ride in apply', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const html = h.get('adguard-sync-results').innerHTML;
  assert.match(html, /data-adguard-route-domain="old\.example"[^>]*>PROXY</);
  assert.match(html, /data-adguard-route-domain="conflict\.example"[^>]*>Blocked</);

  vm.runInContext(`adGuardSyncSetRoute('old.example', 'direct')`, h.context);
  const fromVM = (expression) => JSON.parse(vm.runInContext(`JSON.stringify(${expression})`, h.context));
  assert.ok(fromVM('adGuardSyncImportList()').includes('old.example'));
  assert.equal(fromVM(`adGuardSyncDomainActions.get('old.example')`), 'direct');

  await vm.runInContext('applyAdGuardSync()', h.context);
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.ok(call, 'apply was sent');
  assert.ok(call.body.import.includes('old.example'));
  assert.equal(call.body.domain_actions['old.example'], 'direct');
  assert.equal(call.body.removal_mode, 'keep');
  assert.equal(call.body.acknowledge_risk, true);
});

test('a changed source text forces a fresh full preview instead of applying', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  h.get('adguard-sync-source-text').value = '[/changed.example/]https://host:8443/dns-query\n';
  await vm.runInContext('applyAdGuardSync()', h.context);
  assert.equal(h.calls.some(c => c.path.endsWith('/apply')), false);
  assert.equal(h.calls.filter(c => c.path.endsWith('/preview')).length, 2);
  assert.ok(h.toasts.some(t => /source changed/i.test(t.message)));
});

test('add-only flow ignores stale removal controls and keeps existing routes', async () => {
  const h = harness(preview);
  h.values['adguard-sync-removal-mode'] = 'mirror';
  h.checked['adguard-sync-confirm-mirror'] = true;
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  await vm.runInContext('applyAdGuardSync()', h.context);
  const call = h.calls.find(c => c.path.endsWith('/apply'));
  assert.ok(call, 'missing domains are added');
  assert.equal(call.body.removal_mode, 'keep');
  assert.equal(call.body.confirm_mirror, false);
  assert.deepEqual(call.body.import, ['new.example', '*.new.test'], 'covered and conflict rows are not written');
  assert.equal(h.calls.some(c => c.path.endsWith('/apply') && c.body.removal_mode !== 'keep'), false);
});

test('backend notices stay in the API but are never rendered in the card', async () => {
  const h = harness({
    ...preview,
    notices: [
      { kind: 'upstream', title: 'External upstream targets', message: 'These targets are not this DigitDNS server: 77.88.8.8.' },
      { kind: 'broad', title: 'High-traffic domains', message: '3 high-traffic domains are included.' },
      { kind: 'unchanged', title: 'What will not change', message: 'Rules outside this review stay put.' },
    ],
    warnings: ['upstream 77.88.8.8 is not this DigitDNS server; its domains can be imported'],
  });
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const rendered = h.get('adguard-sync-results').innerHTML + h.get('adguard-sync-summary').innerHTML;
  for (const text of [
    'External upstream targets', 'High-traffic domains', 'What will not change',
    'already covered by your DigitDNS rules', 'Rules outside this review stay put',
  ]) {
    assert.doesNotMatch(rendered, new RegExp(text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
  }
  assert.doesNotMatch(source, /renderAdGuardSyncNotices|adGuardSyncNoticeKind|adguard-sync-notices/);
});

test('bulk controls select and clear every importable row and stage routes', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  await vm.runInContext('adGuardSyncBulkImports(false)', h.context);
  assert.deepEqual(h.get('adguard-sync-results').innerHTML.match(/data-adguard-import="[^"]+"[^>]* checked>/g) || [], [], 'clears imports');
  await vm.runInContext('adGuardSyncBulkImports(true)', h.context);
  assert.equal((h.get('adguard-sync-results').innerHTML.match(/data-adguard-import="[^"]+"[^>]* checked>/g) || []).length, 2);
  await vm.runInContext("adGuardSyncBulkStatus('block')", h.context);
  const fromVM = (expression) => JSON.parse(vm.runInContext(`JSON.stringify(${expression})`, h.context));
  assert.deepEqual(fromVM('Array.from(adGuardSyncDomainActions.entries())'),
    [['new.example', 'block'], ['*.new.test', 'block']]);
});

test('a re-preview after staging routes keeps manual selections and actions', async () => {
  const h = harness(preview);
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  vm.runInContext(`
    adGuardSyncSelectedImport = new Set(['new.example']);
    adGuardSyncDomainActions.set('new.example', 'direct');
    adGuardSyncImportTouched = true;
  `, h.context);
  await vm.runInContext('previewAdGuardSync()', h.context);
  const fromVM = (expression) => JSON.parse(vm.runInContext(`JSON.stringify(${expression})`, h.context));
  assert.deepEqual(fromVM('Array.from(adGuardSyncSelectedImport)'), ['new.example']);
  assert.deepEqual(fromVM('Array.from(adGuardSyncDomainActions.entries())'), [['new.example', 'direct']]);
});

test('choosing New source clears the form and the dropdown', async () => {
  const h = harness(preview);
  vm.runInContext(`
    adGuardSyncSources = [{ id: 'src-1', name: 'Norwich', text: '[/x.example/]dns', enabled: true }];
    adGuardSyncSetSource('src-1');
    adGuardSyncSetSource('');
  `, h.context);
  assert.equal(h.get('adguard-sync-source-select').value, '');
  assert.equal(h.get('adguard-sync-source-name').value, '');
  assert.equal(h.get('adguard-sync-source-text').value, '');
  assert.match(h.get('adguard-sync-source-selected').innerHTML, /New source/);
});

test('blank display names are auto-named on preview', async () => {
  const h = harness(preview);
  h.get('adguard-sync-source-name').value = '';
  await vm.runInContext('previewAdGuardSync({ resetSelection: true })', h.context);
  const call = h.calls.find(c => c.path.endsWith('/preview'));
  assert.match(call.body.name, /^AdGuard Home — \d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
  assert.equal(h.get('adguard-sync-source-name').value, call.body.name);
});

test('saved-source dropdown shows favicon, domain count, date and checksum', async () => {
  const h = harness(preview);
  vm.runInContext(`
    adGuardSyncSources = [{
      id: 'src-1',
      name: 'Norwich',
      text: '[/example.com/other.example/]https://host:8443/dns-query\\n',
      updated_at: '2026-11-21T09:41:00Z',
      last_checksum: 'deadbeefcafe',
      enabled: false,
    }];
    adGuardSyncSourceOpen = true;
    renderAdGuardSyncSourcePicker();
  `, h.context);
  const selected = h.get('adguard-sync-source-selected').innerHTML;
  assert.match(selected, /src="\/img\/providers\/adguard\.png"/);
  assert.match(selected, /Norwich/);
  const options = h.get('adguard-sync-source-options').innerHTML;
  assert.match(options, /data-adguard-source="src-1"/);
  assert.match(options, /src="\/img\/providers\/adguard\.png"/);
  assert.match(options, /2 domains/);
  assert.match(options, /2026-11-21 09:41/);
  assert.match(options, /deadbeef/);
  assert.match(options, /title="Checksum deadbeefcafe"/);
  assert.match(options, /data-adguard-source="src-1"[^>]*aria-selected="true"/, 'the loaded source is active');
  assert.doesNotMatch(options, /Enabled|Disabled/, 'saved lists do not show an enabled state');
});

test('Delete uses the AdGuard list confirmation wording', async () => {
  const h = harness(preview);
  await vm.runInContext('deleteAdGuardSyncSource()', h.context);
  assert.equal(h.confirms.length, 1);
  assert.equal(h.confirms[0].title, 'Delete AdGuard List?');
  assert.equal(h.confirms[0].message, 'Domains already imported into DigitDNS are kept.');
  assert.equal(h.confirms[0].confirmText, 'DELETE LIST');
});

test('the card is compact and has no adblock, send-back, note column or upstream picker', () => {
  const page = readFileSync(resolve(__dirname, '../../web/index.html'), 'utf8');
  const cardStart = page.indexOf('<div id="adguard-sync-card"');
  const cardEnd = page.indexOf('</section>', cardStart);
  assert.ok(cardStart > 0 && cardEnd > cardStart, 'importer card markers are stale');
  const html = page.slice(cardStart, cardEnd);
  assert.match(html, /class="panel-card-header ag-card-title">\s*<i data-feather="download"[^>]*><\/i> AdGuard Upstream List Import/);
  assert.match(html, /id="adguard-sync-status" class="ag-pill ag-pill-neutral"/);
  assert.match(html, /id="adguard-sync-source-toggle"/);
  assert.match(html, /id="adguard-sync-source-menu"/);
  assert.match(html, /id="adguard-sync-source-select"[^>]*aria-label="Active saved source"/);
  assert.match(html, /id="adguard-sync-source-text"[^>]*rows="12"/);
  assert.doesNotMatch(html, /adguard-sync-enabled|Enabled source/);
  assert.doesNotMatch(html, /ag-steps|ag-field-label|ag-fineprint|Paste or pick a source|Review routes|Apply staged rows/);
  assert.match(html, /class="block mb-1 text-\[11px\] text-slate-400">Saved Lists<\/span>/);
  assert.match(html, /id="adguard-sync-source-toggle"\s+class="ag-source-toggle panel-field w-full font-mono"/);
  assert.match(html, /id="adguard-sync-source-name" class="panel-field w-full font-mono"/);
  assert.match(html, /text-slate-400">List Name<\/span>/);
  assert.doesNotMatch(html, /text-slate-400">Saved List Name<\/span>|text-slate-400">Display name<\/span>/);
  assert.match(html, /id="adguard-sync-selected-only" class="domain-filter-chip" aria-pressed="false">Selected Only<\/button>/);
  assert.match(html, /id="adguard-sync-hide-covered" class="domain-filter-chip" aria-pressed="false">Already in DigitDNS<\/button>/);
  assert.match(html, /class="ag-filter-line flex items-center gap-2"/);
  assert.match(html, /class="proxied-search-box">\s*<input type="text" id="adguard-sync-filter"[\s\S]*?class="panel-field panel-field-sm panel-field-icon w-full font-mono"/);
  assert.match(html, /<i data-feather="search" class="field-icon w-3\.5 h-3\.5 text-slate-500"><\/i>/);
  // The top controls use the same full-size fields and buttons as the Domains
  // list above, with the same icon-bearing neutral/danger buttons.
  assert.doesNotMatch(html, /adguard-sync-load-btn|>Load</);
  assert.doesNotMatch(html, /adguard-sync-select-import-all|adguard-sync-clear-import/, 'select-all/clear-import buttons are removed');
  assert.match(html, /id="adguard-sync-delete-btn" class="panel-btn panel-btn-danger shrink-0"[\s\S]*?data-feather="trash-2"/);
  assert.match(html, /id="adguard-sync-save-btn" class="panel-btn panel-btn-ghost shrink-0"/);
  assert.match(html, /id="adguard-sync-preview-btn" class="panel-btn panel-btn-primary shrink-0"/);
  assert.match(html, /data-feather="save"/);
  assert.match(html, /data-feather="play"/);
  const sourceControls = html.slice(
    html.indexOf('<div class="ag-source-controls">'),
    html.indexOf('<select id="adguard-sync-source-select"'),
  );
  for (const id of ['adguard-sync-source-toggle', 'adguard-sync-delete-btn', 'adguard-sync-save-btn', 'adguard-sync-preview-btn']) {
    assert.ok(sourceControls.includes(`id="${id}"`), `${id} must share the source picker row`);
  }
  for (const status of ['proxy', 'direct', 'block']) {
    assert.match(html, new RegExp(`class="domain-filter-chip" data-adguard-bulk-status="${status}"`));
  }
  assert.match(html, /id="adguard-sync-apply-btn" class="panel-btn panel-btn-primary shrink-0"/);
  assert.doesNotMatch(html, /class="panel-chip"/);
  assert.doesNotMatch(html, /class="ag-legend"|Each row has its own route|Add to DigitDNS<\/span>|Conflict<\/span>|Broad<\/span>/);
  assert.doesNotMatch(html, /adguard-sync-ack-risk|Include broad domains/);
  assert.match(html, /Only listed domains are changed\. All other DigitDNS routes stay as they are\./);
  assert.match(html, /Add Missing Domains/);
  assert.match(html, /Save List/);
  assert.doesNotMatch(html, /Save source/);
  assert.match(html, /Apply Route/);
  assert.doesNotMatch(html, /Selected rows:/);
  assert.doesNotMatch(html, /adguard-sync-removal-mode|adguard-sync-confirm-mirror|Removals|Mirror this source|This review only/);
  assert.doesNotMatch(html, /Include broad and conflicting domains/);
  assert.doesNotMatch(html, /adguard-sync-source-url|Refresh URL/);
  assert.doesNotMatch(html, /Generated AdGuard block/);
  assert.doesNotMatch(html, /adguard-sync-upstream|Managed upstream for exported rules|managed_upstream/);
  assert.doesNotMatch(html, /adguard-sync-source-cards|adguard-sync-new-btn|adguard-sync-select-managed|adguard-sync-clear-managed/);
  assert.doesNotMatch(html, /Send back to AdGuard|>Note<|Adblock/);
  assert.doesNotMatch(html, /adguard-sync-notices|External upstream|External upstream targets|High-traffic domains|What will not change|What stays the same|Domains outside this preview/);
  assert.doesNotMatch(html, /AdGuard config files are never rewritten/);
  assert.doesNotMatch(html, /I understand mirror mode removes proxied domains this source does not name\./);
  assert.doesNotMatch(html, /I understand conflicts and broad domains need an explicit acknowledgement\./);
  assert.doesNotMatch(html, /Removals are off by default\. Existing rules outside this preview, and handwritten AdGuard sections, are left alone\./);
  assert.doesNotMatch(source, /adGuardSyncAdblockDomains|data-adguard-adblock|adGuardSyncSelectedManaged|managed_selection/);

  const css = readFileSync(resolve(__dirname, '../../web/css/style.css'), 'utf8');
  assert.match(css, /\.ag-pill-add/);
  assert.match(css, /\.ag-row-covered/);
  assert.match(css, /\.ag-row-conflict/);
  assert.match(css, /\.ag-source-menu/);
  assert.match(css, /\.ag-apply-row/);
  assert.match(css, /\.panel-card-header\.ag-card-title\s*\{[^}]*text-transform:\s*none/);
  assert.match(css, /\.badge-proxy/);
  assert.doesNotMatch(css, /\.stream-action-menu\.adguard-route-menu[^{]*\{[^}]*display:\s*none/);
  assert.match(css, /\.stream-action-scopes\s*\{/);
  assert.match(css, /\.stream-action-target\s*\{/);
  assert.match(css, /\.ag-toolbar\s*\{[^}]*flex-wrap:\s*nowrap/);
  assert.match(css, /\.ag-filter-line\s*\{/);
  assert.match(css, /\.proxied-search-box\s*\{[^}]*position:\s*relative/);
  assert.doesNotMatch(css, /\.ag-search-box/);
  assert.match(css, /\.domain-filter-chip\s*\{/);
  assert.match(css, /\.ag-table-scroll \.proxied-col-domain\s*\{[^}]*width:\s*13rem/);
  assert.match(css, /\.ag-col-upstream\s*\{[^}]*width:\s*21rem/);
  assert.doesNotMatch(css, /\.ag-col-status/, 'the status column is gone');
  assert.doesNotMatch(css, /\.ag-legend|\.ag-status-menu|\.ag-status-options|\.ag-status-option|\.ag-source-cards|\.ag-upstream-menu|\.ag-managed|\.ag-notices|\.ag-notice/);
  assert.doesNotMatch(css, /\.ag-steps|\.ag-field-label|\.ag-fineprint/);
  assert.match(css, /\.ag-source-controls\s*\{[^}]*gap:\s*8px/);
  assert.match(source, /data-adguard-route-domain/);
  assert.match(source, /streamActionMode === 'adguard'/);
  assert.match(source, /function adGuardSyncScopeTargets\(domain, scope\)/);
  assert.match(source, /adGuardSyncScopeTargets\(domain, b\.dataset\.scope/);
  assert.match(source, /updateStreamActionScopes\(streamActionDomain\)/);
  assert.match(source, /streamActionScopeMemory\.set\(streamActionDomain, streamActionScope\)/);
  assert.match(source, /adGuardSyncSetRoute\(streamActionDomain, actionBtn\.dataset\.action \|\| '', targets\)/);
});
