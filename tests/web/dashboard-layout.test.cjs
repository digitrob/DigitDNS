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
  assert.deepEqual(cards.qps, { x: 0, y: 0, w: 2, h: 4 });
  assert.deepEqual(cards.traffic, { x: 10, y: 0, w: 2, h: 4 });
  assert.equal(new Set(Object.values(cards).map((c) => c.y)).size, 1);
});

test('the legacy order/width/height document migrates to x/y/w/h', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout(
    { order: { metrics: ['qps', 'cache'] }, cards: { qps: { w: 2, h: 4 }, cache: { w: 2, h: 4 } } },
    { metrics: ['qps', 'cache'], main: [] },
  )`)));
  assert.equal(layout.version, 2);
  assert.deepEqual(layout.grids.metrics.cards.qps, { x: 0, y: 0, w: 4, h: 8 });
  assert.deepEqual(layout.grids.metrics.cards.cache, { x: 4, y: 0, w: 4, h: 8 });
});

test('a taller card does not change its neighbour height', () => {
  const run = fixture();
  const cards = JSON.parse(JSON.stringify(run(`(() => {
    const cards = {
      a: { x: 0, y: 0, w: 2, h: 10 },
      b: { x: 2, y: 0, w: 2, h: 3 },
    };
    dashCompact(cards, ['a', 'b'], '');
    return cards;
  })()`)));
  assert.equal(cards.a.h, 10);
  assert.equal(cards.b.h, 3);
  assert.equal(cards.b.y, 0);
});

test('a card dropped on an occupied cell pushes the other card down', () => {
  const run = fixture();
  const cards = JSON.parse(JSON.stringify(run(`(() => {
    const cards = {
      a: { x: 0, y: 0, w: 4, h: 10 },
      b: { x: 0, y: 0, w: 4, h: 3 },
    };
    dashCompact(cards, ['a', 'b'], 'b');
    return cards;
  })()`)));
  assert.equal(cards.b.y, 0);
  assert.equal(cards.a.y, 3);
});

test('minimum sizes are enforced for a tiny chart', () => {
  const run = fixture();
  const card = JSON.parse(JSON.stringify(run(`dashClampCard({ x: -5, y: -2, w: 99, h: 0 }, 'chart')`)));
  assert.deepEqual(card, { x: 0, y: 0, w: 12, h: 6 });
});

test('the main defaults preserve the chart / latency / racer arrangement', () => {
  const run = fixture();
  const layout = JSON.parse(JSON.stringify(run(`dashNormalizeLayout({}, { metrics: [], main: ${JSON.stringify(main)} })`)));
  assert.deepEqual(layout.grids.main.cards.chart, { x: 0, y: 0, w: 8, h: 10 });
  assert.deepEqual(layout.grids.main.cards.latency, { x: 0, y: 10, w: 8, h: 7 });
  assert.deepEqual(layout.grids.main.cards.upstream, { x: 8, y: 0, w: 4, h: 17 });
});
