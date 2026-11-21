const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The Domains page offers icon-library suggestions for a word. A domain used to
// suppress them entirely, and a slow response to an earlier edit could repaint
// the choices after the field had changed. These tests lift the shipped helpers
// rather than restating the rules, so a future edit cannot pass with a copy.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');

function functionBody(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `${name} is missing from app.js`);
  const open = source.indexOf('{', start);
  assert.ok(open > start, `${name} has no body`);
  let depth = 0;
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++;
    else if (source[i] === '}') {
      depth--;
      if (depth === 0) return source.slice(open, i + 1);
    }
  }
  assert.fail(`${name} is unterminated`);
}

function makeBox() {
  const classes = new Set(['hidden']);
  return {
    innerHTML: '',
    classList: {
      add: (c) => classes.add(c),
      remove: (c) => classes.delete(c),
      contains: (c) => classes.has(c),
    },
  };
}

function iconHarness() {
  const from = source.indexOf('function serviceIconURL(');
  const to = source.indexOf('function renderServices(');
  assert.ok(from >= 0 && to > from, 'could not find the icon helpers in app.js');
  const box = makeBox();
  const urls = [];
  let responder = () => ({ ok: true, json: async () => ({ icons: ['logos:netflix'] }) });
  const context = vm.createContext({
    document: { getElementById: (id) => (id === 'service-icon-results' ? box : null) },
    api: (path) => path,
    authToken: 'test-token',
    clearTimeout: () => {},
    escapeHTML: (v) => String(v),
    fetch: async (url) => {
      urls.push(url);
      return responder(url);
    },
  });
  vm.runInContext(source.slice(from, to), context);
  return {
    box,
    urls,
    query: (value) => vm.runInContext(`serviceIconQuery(${JSON.stringify(value)})`, context),
    setResponder: (fn) => { responder = fn; },
    search: (value) => vm.runInContext(`searchServiceIcons(${JSON.stringify(value)})`, context),
    hide: () => vm.runInContext('hideIconResults()', context),
  };
}

test('a service name, domain or URL is reduced to a searchable icon term', () => {
  const body = functionBody('serviceIconQuery');
  const context = vm.createContext({});
  vm.runInContext(`function serviceIconQuery(raw) ${body}`, context);
  const query = (value) => vm.runInContext(`serviceIconQuery(${JSON.stringify(value)})`, context);

  assert.equal(query('Netflix'), 'netflix');
  assert.equal(query('BBC iPlayer'), 'bbc iplayer');
  assert.equal(query('netflix.com'), 'netflix');
  assert.equal(query('https://www.netflix.com/browse'), 'netflix');
  assert.equal(query('ads.netflix.co.uk'), 'netflix');
  assert.equal(query('example.gov.au'), 'example');
  // An already-chosen image, or an address with no name, is not a search term.
  assert.equal(query('https://api.iconify.design/logos/netflix.svg'), '');
  assert.equal(query('https://icons.duckduckgo.com/ip3/netflix.com.ico'), '');
  assert.equal(query('127.0.0.1'), '');
  assert.equal(query(''), '');
});

test('a domain still opens icon suggestions', async () => {
  const h = iconHarness();
  await h.search('netflix.com');
  assert.deepEqual(h.urls, ['/api/icons/search?q=netflix']);
  assert.equal(h.box.classList.contains('hidden'), false);
  assert.match(h.box.innerHTML, /api\.iconify\.design\/logos\/netflix\.svg/);
});

test('hiding the choices invalidates a response already in flight', async () => {
  const h = iconHarness();
  let release;
  h.setResponder(() => new Promise((resolve) => { release = resolve; }));
  const stale = h.search('netflix');
  // The operator changed the field (or picked a choice) before the reply landed.
  h.hide();
  release({ ok: true, json: async () => ({ icons: ['logos:netflix'] }) });
  await stale;

  assert.equal(h.box.classList.contains('hidden'), true);
  assert.equal(h.box.innerHTML, '');
});

test('the input handler cancels the debounce and stale searches before queuing a new one', () => {
  assert.match(source, /hideIconResults\(\);\s*\n\s*iconSearchTimer = setTimeout\(\(\) => searchServiceIcons\(value\), 350\)/);
  assert.match(source, /iconSearchSeq\+\+;\s*\n\s*clearTimeout\(iconSearchTimer\)/);
  assert.match(source, /const q = serviceIconQuery\(raw\);/);
});
