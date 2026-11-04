const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const start = source.indexOf('// DASH_LAYOUT_PURE_START');
const end = source.indexOf('// DASH_LAYOUT_PURE_END');
assert.ok(start >= 0 && end > start, 'layout helpers are missing their test markers');

function fixture() {
  const context = vm.createContext({});
  vm.runInContext(source.slice(start, end), context);
  return (code) => vm.runInContext(code, context);
}

const metrics = ['qps', 'cache', 'sni', 'cpu', 'ram', 'traffic'];
const main = ['chart', 'upstream', 'latency'];

test('default metrics sit on one aligned 12-column row', () => {
  const run = fixture();
  const cards = JSON.parse(JSON.stringify(run(`dashDefaultCards('metrics', ${JSON.stringify(metrics)})`)));
  assert.deepEqual(cards.qps, { x: 0, y: 0, w: 2, h: 7 });
  assert.deepEqual(cards.traffic, { x: 10, y: 0, w: 2, h: 7 });
  assert.equal(new Set(Object.values(cards).map((c) => c.y)).size, 1);
});

test('the legacy order/width/height document migrates to the 12px v3 grid', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout(
    { order: { metrics: ['qps', 'cache'] }, cards: { qps: { w: 2, h: 4 }, cache: { w: 2, h: 4 } } },
    { metrics: ['qps', 'cache'], main: [] },
  )`)));
  assert.equal(layout.version, 3);
  assert.deepEqual(layout.grids.metrics.cards.qps, { x: 0, y: 0, w: 4, h: 16 });
  assert.deepEqual(layout.grids.metrics.cards.cache, { x: 4, y: 0, w: 4, h: 16 });
});

test('a v2 layout migrates to the 12px grid', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout(
    { version: 2, grids: { metrics: { cards: {
      qps: { x: 0, y: 0, w: 2, h: 4 },
      cache: { x: 2, y: 0, w: 2, h: 4 },
    } } } },
    { metrics: ['qps', 'cache'], main: [] },
  )`)));
  assert.equal(layout.version, 3);
  assert.deepEqual(layout.grids.metrics.cards.qps, { x: 0, y: 0, w: 2, h: 7 });
  assert.deepEqual(layout.grids.metrics.cards.cache, { x: 2, y: 0, w: 2, h: 7 });
});

test('a saved v3 layout keeps deliberate gaps and positions', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout(
    { version: 3, grids: { metrics: { cards: {
      qps: { x: 0, y: 10, w: 2, h: 7 },
      cache: { x: 2, y: 0, w: 2, h: 7 },
    } } } },
    { metrics: ['qps', 'cache'], main: [] },
  )`)));
  assert.deepEqual(layout.grids.metrics.cards.qps, { x: 0, y: 10, w: 2, h: 7 });
  assert.deepEqual(layout.grids.metrics.cards.cache, { x: 2, y: 0, w: 2, h: 7 });
});

test('a card saved in another grid stays there across reloads', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout(
    { version: 3, grids: {
      main: { cards: {
        chart: { x: 0, y: 0, w: 8, h: 19 },
        'history-top-blocked': { x: 8, y: 19, w: 4, h: 11 },
      } },
      history: { cards: {} },
    } },
    { metrics: [], main: ['chart'], history: ['history-top-blocked'] },
  )`)));
  assert.deepEqual(layout.grids.main.cards['history-top-blocked'], { x: 8, y: 19, w: 4, h: 11 });
  assert.equal(layout.grids.history.cards['history-top-blocked'], undefined);
});

test('a taller card does not change its neighbour height', () => {
  const run = fixture();
  const cards = JSON.parse(JSON.stringify(run(`(() => {
    const cards = {
      a: { x: 0, y: 0, w: 2, h: 20 },
      b: { x: 2, y: 0, w: 2, h: 6 },
    };
    dashCompact(cards, ['a', 'b'], '');
    return cards;
  })()`)));
  assert.equal(cards.a.h, 20);
  assert.equal(cards.b.h, 6);
  assert.equal(cards.b.y, 0);
});

test('a card dropped on an occupied cell pushes the other card down when compacting', () => {
  const run = fixture();
  const cards = JSON.parse(JSON.stringify(run(`(() => {
    const cards = {
      a: { x: 0, y: 0, w: 4, h: 20 },
      b: { x: 0, y: 0, w: 4, h: 6 },
    };
    dashCompact(cards, ['a', 'b'], 'b');
    return cards;
  })()`)));
  assert.equal(cards.b.y, 0);
  assert.equal(cards.a.y, 6);
});

test('minimum sizes are enforced for a tiny chart', () => {
  const run = fixture();
  const card = JSON.parse(JSON.stringify(run(`dashClampCard({ x: -5, y: -2, w: 99, h: 0 }, 'chart')`)));
  assert.deepEqual(card, { x: 0, y: 0, w: 12, h: 10 });
});

test('the main defaults preserve the chart / latency / racer arrangement', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout({}, { metrics: [], main: ${JSON.stringify(main)} })`)));
  assert.deepEqual(layout.grids.main.cards.chart, { x: 0, y: 0, w: 8, h: 19 });
  assert.deepEqual(layout.grids.main.cards.latency, { x: 0, y: 19, w: 8, h: 13 });
  assert.deepEqual(layout.grids.main.cards.upstream, { x: 8, y: 0, w: 4, h: 33 });
});

test('history cards get an aligned KPI row, a wide chart and ranked tables', () => {
  const run = fixture();
  const history = [
    'history-total', 'history-blocked', 'history-proxied',
    'history-cache', 'history-clients', 'history-latency',
    'history-chart', 'history-top-domains', 'history-top-blocked',
    'history-top-clients', 'history-top-upstreams', 'history-top-protocols',
    'history-top-relay',
  ];
  const layout = JSON.parse(JSON.stringify(run(
    `dashNormalizeLayout({}, { metrics: [], main: [], history: ${JSON.stringify(history)} })`,
  )));
  assert.deepEqual(layout.grids.history.cards['history-total'], { x: 0, y: 0, w: 2, h: 7 });
  assert.deepEqual(layout.grids.history.cards['history-latency'], { x: 10, y: 0, w: 2, h: 7 });
  assert.deepEqual(layout.grids.history.cards['history-chart'], { x: 0, y: 7, w: 12, h: 19 });
  assert.deepEqual(layout.grids.history.cards['history-top-domains'], { x: 0, y: 26, w: 4, h: 11 });
  assert.deepEqual(layout.grids.history.cards['history-top-relay'], { x: 8, y: 37, w: 4, h: 11 });
});

test('history chart and tables keep their minimum sizes', () => {
  const run = fixture();
  assert.deepEqual(
    JSON.parse(JSON.stringify(run(`dashClampCard({ x: 0, y: 0, w: 1, h: 1 }, 'history-chart')`))),
    { x: 0, y: 0, w: 4, h: 10 },
  );
  assert.deepEqual(
    JSON.parse(JSON.stringify(run(`dashClampCard({ w: 1, h: 1 }, 'history-top-clients')`))),
    { x: 0, y: 0, w: 2, h: 8 },
  );
});

test('free placement refuses an overlapping or out-of-bounds drop', () => {
  const run = fixture();
  assert.equal(run(`dashCardFits({ x: 2, y: 0, w: 2, h: 2 }, { a: { x: 0, y: 0, w: 2, h: 2 } }, 'b')`), true);
  assert.equal(run(`dashCardFits({ x: 1, y: 0, w: 2, h: 2 }, { a: { x: 0, y: 0, w: 2, h: 2 } }, 'b')`), false);
  assert.equal(run(`dashCardFits({ x: 11, y: 0, w: 2, h: 2 }, {}, 'b')`), false);
  assert.equal(run(`dashCardFits({ x: 0, y: 0, w: 2, h: 2 }, { a: { x: 0, y: 0, w: 2, h: 2 } }, 'a')`), true);
});

test('resize math works from every edge and corner', () => {
  const run = fixture();
  const base = `{ id: 'chart', x: 2, y: 4, w: 6, h: 10 }`;
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 'e', 2, 0)`))), { x: 2, y: 4, w: 8, h: 10 });
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 's', 0, 3)`))), { x: 2, y: 4, w: 6, h: 13 });
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 'w', -2, 0)`))), { x: 0, y: 4, w: 8, h: 10 });
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 'n', 0, -2)`))), { x: 2, y: 2, w: 6, h: 12 });
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 'se', 2, 2)`))), { x: 2, y: 4, w: 8, h: 12 });
  assert.deepEqual(JSON.parse(JSON.stringify(run(`dashResizeRect(${base}, 'nw', -2, -2)`))), { x: 0, y: 2, w: 8, h: 12 });
});
