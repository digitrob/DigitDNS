const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// A scope change must keep the row where it is: replacePatterns rewrites the list in
// the slot of the first removed pattern, and only appends when the row is not there.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const code = source.slice(
  source.indexOf('// replacePatterns rewrites one rule list'),
  source.indexOf('// streamRowStamp returns'),
);

function replace(list, remove, add) {
  const context = vm.createContext({});
  vm.runInContext(code, context);
  return [...vm.runInContext('replacePatterns', context)(list, remove, add)];
}

test('keeps the slot of the replaced pattern', () => {
  assert.deepEqual(
    replace(['x.example', 'a.b.c', 'y.example'], ['a.b.c'], ['*.a.b.c']),
    ['x.example', '*.a.b.c', 'y.example'],
  );
});

test('replacing a wildcard with its exact host keeps the slot', () => {
  assert.deepEqual(
    replace(['x.example', '*.a.b.c'], ['*.a.b.c'], ['a.b.c']),
    ['x.example', 'a.b.c'],
  );
});

test('host plus wildcard stays in the old slot', () => {
  assert.deepEqual(
    replace(['x.example', 'a.b.c', 'y.example'], ['a.b.c'], ['a.b.c', '*.a.b.c']),
    ['x.example', 'a.b.c', '*.a.b.c', 'y.example'],
  );
});

test('appends when the row is not in the list', () => {
  assert.deepEqual(replace(['x.example'], [], ['*.a.b.c']), ['x.example', '*.a.b.c']);
});

test('does not duplicate a pattern that is already present', () => {
  assert.deepEqual(
    replace(['x.example', '*.a.b.c', 'y.example'], ['a.b.c'], ['*.a.b.c']),
    ['x.example', '*.a.b.c', 'y.example'],
  );
});
