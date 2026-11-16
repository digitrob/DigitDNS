const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

const root = resolve(__dirname, '../..');

// A dangling reference to a constant that was deleted somewhere else in the file is
// invisible to `node --check`: an undefined identifier is a runtime ReferenceError,
// not a syntax error, and the panel only reports it in the console of whoever
// clicks that control. v5.8.12 deleted STREAM_ACTION_LOG_BADGE while adding the
// stream info button and left one reference behind, so every Proxy/Block/Direct
// quick action threw after mutating the in-memory rules and before saving them —
// four releases in which the action menu silently did nothing.
//
// This is the smallest static check that catches that shape: strip comments and
// string/template text, collect every declared name, then look for SCREAMING_CASE
// identifiers that are used but never declared. Lowercase and CamelCase names are
// deliberately out of scope — the pattern only covers the module-level constant
// style this codebase writes in, which is where the references are most likely to
// outlive their definitions.
function stripCode(src) {
  const out = [];
  const modes = ['code'];
  const depths = [0];
  let i = 0;
  const n = src.length;
  const emit = (ch) => out.push(ch);

  while (i < n) {
    const mode = modes[modes.length - 1];
    const c = src[i];

    if (mode === 'template') {
      if (c === '\\') {
        emit(' ');
        i++;
        if (i < n) { emit(src[i] === '\n' ? '\n' : ' '); i++; }
        continue;
      }
      if (c === '`') { emit(' '); i++; modes.pop(); depths.pop(); continue; }
      if (c === '$' && src[i + 1] === '{') {
        emit('  ');
        i += 2;
        modes.push('code');
        depths.push(0);
        continue;
      }
      emit(c === '\n' ? '\n' : ' ');
      i++;
      continue;
    }

    if (c === '/' && src[i + 1] === '/') {
      while (i < n && src[i] !== '\n') { emit(' '); i++; }
      continue;
    }
    if (c === '/' && src[i + 1] === '*') {
      emit('  ');
      i += 2;
      while (i < n && !(src[i] === '*' && src[i + 1] === '/')) {
        emit(src[i] === '\n' ? '\n' : ' ');
        i++;
      }
      if (i < n) { emit('  '); i += 2; }
      continue;
    }
    if (c === '"' || c === "'") {
      const quote = c;
      emit(' ');
      i++;
      while (i < n && src[i] !== quote) {
        if (src[i] === '\\') { emit(' '); i++; }
        if (i < n) { emit(src[i] === '\n' ? '\n' : ' '); i++; }
      }
      if (i < n) { emit(' '); i++; }
      continue;
    }
    if (c === '`') { emit(' '); i++; modes.push('template'); depths.push(0); continue; }

    if (c === '{') depths[depths.length - 1]++;
    if (c === '}') {
      if (depths[depths.length - 1] === 0 && modes.length > 1) {
        emit(' ');
        i++;
        modes.pop();
        depths.pop();
        continue;
      }
      depths[depths.length - 1]--;
    }
    emit(c);
    i++;
  }
  return out.join('');
}

// Names the browser and the platform provide. Anything else in SCREAMING_CASE has
// to be declared in the file: app.js is one script, and every constant it reads is
// written in it.
const PLATFORM = new Set([
  'JSON', 'URL', 'DOM', 'API', 'ID', 'UI', 'CSS', 'HTML', 'SVG', 'PNG', 'CSV', 'JS',
  'MS', 'KB', 'MB', 'GB', 'UTC', 'GMT', 'IP', 'DNS', 'TLS', 'SNI', 'CPU', 'RAM',
  'QPS', 'TTL', 'PTR', 'NS', 'MX', 'TXT', 'SOA', 'SRV', 'DS', 'ANY', 'TYPE',
]);

test('app.js references no undeclared constant', () => {
  const source = readFileSync(resolve(root, 'web/js/app.js'), 'utf8');
  const code = stripCode(source);

  const declared = new Set();
  for (const m of code.matchAll(/\b(?:const|let|var|function|class)\s+([A-Za-z_$][\w$]*)/g)) {
    declared.add(m[1]);
  }
  for (const m of code.matchAll(/\b(?:const|let|var)\s*\{([^}]*)\}/g)) {
    for (const part of m[1].split(',')) {
      const name = part.split(':').pop().trim().split('=')[0].trim();
      if (name) declared.add(name);
    }
  }

  // Guard the scan itself: a stripper that swallowed the file would otherwise pass
  // by finding nothing at all.
  assert.ok(declared.size > 500, `only ${declared.size} declarations found — the scanner is not reading app.js`);

  const missing = new Map();
  for (const m of code.matchAll(/(^|[^.\w$])([A-Z][A-Z0-9_]{2,})\b(?!\s*:)/g)) {
    const name = m[2];
    if (declared.has(name) || PLATFORM.has(name)) continue;
    if (!missing.has(name)) {
      missing.set(name, code.slice(0, m.index + m[1].length).split('\n').length);
    }
  }

  const report = [...missing].map(([name, line]) => `${name} (app.js:${line})`).join(', ');
  assert.equal(missing.size, 0,
    `app.js reads identifiers that nothing in the file declares: ${report}. ` +
    'A ReferenceError from one of these aborts its function mid-way, after whatever ' +
    'it had already changed and before it saved.');
});

// The regression this file was written for, stated directly: the quick-action menu
// is the only place these three actions can be chosen from the domain list and the
// live log, so the badge map it stamps the buffer with has to exist.
test('the quick-action badge map covers all three actions', () => {
  const source = readFileSync(resolve(root, 'web/js/app.js'), 'utf8');
  const m = source.match(/const STREAM_ACTION_LOG_BADGE = \{([^}]*)\}/);
  assert.ok(m, 'STREAM_ACTION_LOG_BADGE is missing — every quick action throws before it saves');
  for (const action of ['proxy', 'block', 'direct']) {
    assert.match(m[1], new RegExp(`\\b${action}:\\s*'[A-Z_]+'`),
      `STREAM_ACTION_LOG_BADGE has no entry for ${action}`);
  }
});
