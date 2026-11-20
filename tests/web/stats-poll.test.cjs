const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The dashboard's poll loop is the only consumer of compact=2. Lift the helper
// and updateStats straight out of app.js rather than reimplementing the merge:
// a copy in the test would keep passing while the shipped page drifted.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const from = source.indexOf('function mergeCompactStats(');
const to = source.indexOf('\n// statsFailures counts');
if (from < 0 || to < 0 || to <= from) {
  throw new Error('could not find the stats poll helpers in web/js/app.js');
}
const statsCode = source.slice(from, to);

function harness(full, live) {
  const urls = [];
  const renderedUpstreams = [];
  const elements = new Map();
  const context = vm.createContext({
    authToken: 'test-token',
    api: (path) => path,
    fetch: async (url) => {
      urls.push(url);
      const compact = url.includes('compact=2');
      const body = compact ? live : full;
      return { ok: true, status: 200, json: async () => body };
    },
    document: { querySelector: () => null, visibilityState: 'visible' },
    activeDashTab: 'dashboard',
    statEl: (id) => {
      if (!elements.has(id)) elements.set(id, { textContent: '', className: '' });
      return elements.get(id);
    },
    renderRateLimit() {},
    renderLatency() {},
    renderCacheHealth() {},
    renderProxyDrops() {},
    renderUpstreams: (list) => { renderedUpstreams.push(JSON.parse(JSON.stringify(list))); },
    pushChartData() {},
    markStatsHealth() {},
    formatBytes: () => '',
    fmtWhole: (value) => String(value),
  });
  vm.runInContext('let statsFull = null; let statsPollCount = 0;', context);
  vm.runInContext(statsCode, context);
  return {
    urls,
    elements,
    renderedUpstreams,
    update: () => vm.runInContext('updateStats()', context),
    invalidate: () => vm.runInContext('invalidateStatsSnapshot()', context),
  };
}

const fullPayload = {
  qps: 1,
  total_queries: 10,
  cpu_usage_percent: 12.5,
  system_mem_used_mb: 1800,
  system_mem_total_mb: 4096,
  upstreams: [{ address: '1.1.1.1:53', latency: 10_000_000, latency_p95_ms: 18, measured: true }],
};
const livePayload = {
  qps: 2,
  total_queries: 12,
  upstreams: [{ address: '1.1.1.1:53', latency: 9_000_000, measured: true }],
};

test('the first poll is full and the next one is a compact=2 heartbeat', async () => {
  const h = harness(fullPayload, livePayload);
  await h.update();
  await h.update();
  assert.deepEqual(h.urls, ['/api/stats', '/api/stats?compact=2']);
});

test('a heartbeat keeps slow fields from the last full document', async () => {
  const h = harness(fullPayload, livePayload);
  await h.update();
  await h.update();
  assert.equal(h.elements.get('stat-cpu-percent').textContent, '12.5%');
  assert.equal(h.elements.get('stat-mem-sys-total').textContent, '4096');
  assert.equal(h.elements.get('stat-qps').textContent, '2.0');
});

test('a heartbeat merges upstream rows by address instead of replacing them', async () => {
  const h = harness(fullPayload, livePayload);
  await h.update();
  await h.update();
  assert.deepEqual(h.renderedUpstreams.at(-1), [{
    address: '1.1.1.1:53',
    latency: 9_000_000,
    latency_p95_ms: 18,
    measured: true,
  }]);
});

test('an invalidated snapshot forces a fresh full document', async () => {
  const h = harness(fullPayload, livePayload);
  await h.update();
  await h.update();
  h.invalidate();
  await h.update();
  assert.deepEqual(h.urls, ['/api/stats', '/api/stats?compact=2', '/api/stats']);
});
