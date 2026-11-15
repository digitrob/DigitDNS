const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');

// The live-log status pill is a transport indicator, not a retry log. It used to
// narrate every reconnect — Live → Reconnecting → Polling → Live on each 1s
// backoff — and because the label changes width, the whole toolbar row shifted
// with it. These tests pin the two things that fixed it: degraded states are
// delayed behind a grace timer, and an open-but-idle socket is never relabelled
// "Polling" by a tab switch.

const root = resolve(__dirname, '../..');
const source = readFileSync(resolve(root, 'web/js/app.js'), 'utf8');
const css = readFileSync(resolve(root, 'web/css/style.css'), 'utf8');

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

test('a dropped stream waits out the first backoff before relabelling', () => {
  const degraded = functionBody('markStreamDegraded');
  const firstTimer = degraded.indexOf('setTimeout');
  const firstLabel = degraded.indexOf("setStreamStatus('reconnecting')");
  assert.ok(firstLabel > 0, 'markStreamDegraded must eventually show "Reconnecting…"');
  assert.ok(
    firstTimer >= 0 && firstTimer < firstLabel,
    'the reconnect label must sit inside a timer — a blip that reconnects in 1s would otherwise flash',
  );
  assert.match(degraded, /STREAM_STATUS_GRACE_MS/, 'the grace delay must gate the reconnect label');
  assert.match(degraded, /streamIsOpen\(\)/, 'the delayed label must re-check the socket before painting');
});

test('the error path does not paint a degraded state itself', () => {
  const onerror = source.slice(
    source.indexOf('source.onerror = () =>'),
    source.indexOf('// scheduleStreamReconnect'),
  );
  assert.ok(onerror.length > 0, 'source.onerror is missing');
  assert.match(onerror, /markStreamDegraded\(\)/, 'onerror must go through the hysteresis');
  assert.ok(
    !/setStreamStatus\(\s*'(reconnecting|polling)'/.test(onerror),
    'onerror must not write a degraded label directly',
  );
});

test('an open socket is never sent to the polling fallback by a tab switch', () => {
  const guards = source
    .split('\n')
    .filter((line) => line.includes('else if') && line.includes('startStreamFallback()'));
  assert.equal(guards.length, 2, 'expected the tab-switch and visibility paths');
  for (const line of guards) {
    assert.match(line, /!streamIsOpen\(\)/, 'the fallback must not start while the EventSource is open');
  }
});

test('a frame on an open socket restores Live', () => {
  const activity = functionBody('markStreamActivity');
  assert.match(activity, /if \(streamIsOpen\(\)\) markStreamLive\(\)/, 'a delivered frame must clear a stale Polling label');
});

test('the status pill keeps one width for every label', () => {
  const pill = css.slice(css.indexOf('.stream-status-pill {'), css.indexOf('.stream-status-dot {'));
  assert.match(pill, /min-width:\s*120px/, 'the pill must reserve the widest label');
  assert.match(pill, /justify-content:\s*center/, 'the label must stay centred inside the fixed width');
  assert.match(pill, /flex-shrink:\s*0/, 'the pill must not shrink in the toolbar row');
});
