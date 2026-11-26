const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// A live-log quick action changes the rule, not the answer a past query already
// received. These tests pin the session-only marker: the original badge is kept,
// the row is marked only after a successful save, and the mark follows the query
// id through re-renders instead of a mutable q.action field.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');

function markerHarness() {
  const from = source.indexOf('const streamChangedQueries = new Map();');
  const to = source.indexOf('// A live table that inserts at the top');
  assert.ok(from >= 0 && to > from, 'could not find the stream marker helpers in app.js');
  const context = vm.createContext({ STREAM_BUFFER_MAX: 100 });
  vm.runInContext(source.slice(from, to), context);
  return {
    mark: (q, toAction) => vm.runInContext(
      `markStreamQueryChanged(${JSON.stringify(q)}, ${JSON.stringify(toAction)})`, context,
    ),
    change: (q) => vm.runInContext(`streamQueryChange(${JSON.stringify(q)})`, context),
    clear: () => vm.runInContext('streamChangedQueries.clear()', context),
  };
}

function rowHarness(change) {
  const from = source.indexOf('function queryRowHTML(q) {');
  const to = source.indexOf('\n}\n\n// newQueryRow', from);
  assert.ok(from >= 0 && to > from, 'could not find queryRowHTML in app.js');
  const context = vm.createContext({
    STREAM_ACTION_BADGES: { DIRECT: '<span class="badge badge-direct">DIRECT</span>' },
    blocklistCategory: () => '',
    countryName: () => '',
    currentConfig: { server: {} },
    domainCopyButton: (domain) => `<button type="button" class="domain-copy-btn" data-copy-value="${domain}">copy</button>`,
    escapeHTML: (v) => String(v),
    flagURL: () => '',
    formatLatencyMs: () => '1ms',
    infoButton: '',
    isProxyAction: () => false,
    latencyTierClass: () => 'latency-fast',
    protocolLabel: () => 'UDP',
    protocolPillClass: () => 'proto-udp',
    streamQueryChange: () => change,
  });
  vm.runInContext(source.slice(from, to + 2), context);
  return {
    html: (q) => vm.runInContext(`queryRowHTML(${JSON.stringify(q)})`, context),
  };
}

const row = {
  id: 42,
  domain: 'ads.example',
  client_ip: '127.0.0.1',
  action: 'DIRECT',
  timestamp: '2026-01-01T00:00:00Z',
};

test('a saved decision marks the query id without rewriting its original action', () => {
  const h = markerHarness();
  h.mark(row, 'PROXY');
  // A polling refresh hands the renderer a fresh object with the same id.
  const refreshed = { ...row, action: 'DIRECT', rule_matched: 'Default Direct' };
  const mark = h.change(refreshed);
  assert.equal(mark.from, 'DIRECT');
  assert.equal(mark.to, 'PROXY');
  assert.equal(typeof mark.at, 'number');
  assert.equal(refreshed.action, 'DIRECT');
});

test('a mark falls back to domain, timestamp and client when no id is present', () => {
  const h = markerHarness();
  const noId = { domain: 'one.example', timestamp: 't1', client_ip: '10.0.0.1', action: 'BLOCK' };
  h.mark(noId, 'DIRECT');
  assert.equal(h.change({ ...noId }).to, 'DIRECT');
  assert.equal(h.change({ ...noId, timestamp: 't2' }), undefined);
  h.clear();
  assert.equal(h.change(noId), undefined);
});

test('the marked row keeps its old badge and gains a greyed, labelled button', () => {
  const { html } = rowHarness({ from: 'DIRECT', to: 'PROXY', at: 1 });
  const out = html(row);
  assert.match(out, /class="stream-action-btn stream-action-changed"/);
  assert.match(out, /data-stream-changed="PROXY"/);
  assert.match(out, /rule changed to PROXY/);
  assert.match(out, /<span class="badge badge-direct">DIRECT<\/span>/);
  assert.match(out, /data-domain="ads\.example"/);
});

test('an unmarked row renders the ordinary action button', () => {
  const { html } = rowHarness(null);
  const out = html(row);
  assert.match(out, /class="stream-action-btn"/);
  assert.doesNotMatch(out, /stream-action-changed/);
  assert.doesNotMatch(out, /data-stream-changed/);
});

test('rows are marked after save, only for the clicked domain, never from Explain', () => {
  assert.ok(!/q\.action\s*=\s*logBadge/.test(source), 'a quick action still rewrites query history');
  for (const want of [
    'if (!streamActionExplainSource && logBadge)',
    "if ((q.domain || '') !== streamActionDomain) return;",
    'markStreamQueryChanged(q, logBadge);',
    'streamChangedQueries.clear();',
    'const change = streamQueryChange(q);',
    'class="stream-action-btn${changedClass}"',
  ]) {
    assert.ok(source.includes(want), `app.js no longer contains ${want}`);
  }
  // The mark must be recorded after the save, not in the optimistic rule edit.
  const saveAt = source.indexOf('if (!(await saveRules(true)))', source.indexOf('async function applyStreamAction'));
  const markAt = source.indexOf('markStreamQueryChanged(q, logBadge);', saveAt);
  assert.ok(saveAt >= 0 && markAt > saveAt, 'the marker is written before the save is known to succeed');
});
