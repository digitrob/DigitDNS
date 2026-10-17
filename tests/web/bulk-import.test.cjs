const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// parseDomainList is the whole of the bulk-import feature that can be wrong on its
// own: the rest is a modal that forwards its textarea into this function and merges
// what comes back. So it is lifted straight out of app.js and run in a bare context.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const parserCode = source.slice(source.indexOf('const DOMAIN_RE'), source.indexOf('function renderList'));

function parse(text, existing) {
  const context = vm.createContext({});
  vm.runInContext(parserCode, context);
  return vm.runInContext('parseDomainList', context)(text, existing);
}

test('splits on newlines, commas, semicolons and runs of spaces', () => {
  const { accepted } = parse('a.example.com\nb.example.com, c.example.com; d.example.com  e.example.com');
  assert.deepEqual(accepted, [
    'a.example.com', 'b.example.com', 'c.example.com', 'd.example.com', 'e.example.com',
  ]);
});

test('lowercases, strips the root dot and keeps a leading wildcard', () => {
  const { accepted } = parse('*.EXAMPLE.com.\nApi.Target.NET');
  assert.deepEqual(accepted, ['*.example.com', 'api.target.net']);
});

test('a hash starts a comment that runs to the end of its line', () => {
  const { accepted, invalid } = parse('example.com # keep this\n# whole line ignored\nother.example.net');
  assert.deepEqual(accepted, ['example.com', 'other.example.net']);
  assert.deepEqual(invalid, []);
});

test('drops names already present and repeats inside the blob', () => {
  const { accepted, duplicates } = parse('new.example.com\nold.example.com\nnew.example.com', ['old.example.com']);
  assert.deepEqual(accepted, ['new.example.com']);
  assert.deepEqual(duplicates.sort(), ['new.example.com', 'old.example.com']);
});

test('rejects a bare label, a leading dot, a bad label and a lone wildcard', () => {
  const { accepted, invalid } = parse(['localhost', '.example.com', 'bad_label.example.com', '*.', 'ok.example.com'].join('\n'));
  assert.deepEqual(accepted, ['ok.example.com']);
  assert.deepEqual(invalid.sort(), ['.example.com', '*.', 'bad_label.example.com', 'localhost']);
});

test('an empty or comment-only blob imports nothing', () => {
  const { accepted, duplicates, invalid } = parse('\n  \n# nothing here\n');
  assert.deepEqual(accepted, []);
  assert.deepEqual(duplicates, []);
  assert.deepEqual(invalid, []);
});
