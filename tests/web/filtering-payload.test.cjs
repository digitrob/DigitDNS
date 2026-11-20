const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

// The filter save body has to carry every setting: the endpoint replaces the
// whole record, so dropping one field silently clears an operator's choice.
// Run the literal from app.js instead of a hand-written copy.
const source = readFileSync(resolve(__dirname, '../../web/js/app.js'), 'utf8');
const fnStart = source.indexOf('async function saveFilteringSettings()');
const declStart = source.indexOf('  const enabled =', fnStart);
const validationStart = source.indexOf('  if (!Number.isInteger(ttl)', declStart);
const payloadStart = source.indexOf('  const payload = {', fnStart);
const payloadEnd = source.indexOf('\n  };', payloadStart);
if (fnStart < 0 || declStart < 0 || validationStart < 0 || payloadStart < 0 || payloadEnd < 0) {
  throw new Error('could not find the filtering save payload in web/js/app.js');
}
// Skip the validation block between the field reads and the body: it calls
// showToast/return, which are irrelevant to the serialized shape.
const buildCode = source.slice(declStart, validationStart) + '\n' + source.slice(payloadStart, payloadEnd + 5);

function build(values) {
  const fields = {
    'filtering-enabled-input': { checked: values.enabled },
    'filtering-mode-select': { value: values.mode },
    'filtering-ttl-input': { value: String(values.ttl) },
    'filtering-interval-input': { value: String(values.interval) },
    'filtering-allowlist-input': { value: values.allowlist.join('\n') },
    'filtering-custom-rules-input': { value: values.customRules },
  };
  const context = vm.createContext({
    document: { getElementById: (id) => fields[id] || null },
  });
  vm.runInContext(buildCode, context);
  return JSON.parse(JSON.stringify(vm.runInContext('payload', context)));
}

test('the filter save body keeps every setting and splits the allowlist by line', () => {
  assert.deepEqual(build({
    enabled: true,
    mode: 'nxdomain',
    ttl: 45,
    interval: 12,
    allowlist: [' example.com ', 'ads.test', ''],
    customRules: '||tracker.example^\n',
  }), {
    enabled: true,
    block_mode: 'nxdomain',
    block_ttl: 45,
    update_interval_hours: 12,
    allowlist: ['example.com', 'ads.test'],
    custom_rules: '||tracker.example^\n',
  });
});

test('an unchecked filter still sends false rather than an omitted field', () => {
  const payload = build({
    enabled: false,
    mode: 'null_ip',
    ttl: 10,
    interval: 24,
    allowlist: [],
    customRules: '',
  });
  assert.equal(payload.enabled, false);
  assert.deepEqual(Object.keys(payload), [
    'enabled', 'block_mode', 'block_ttl', 'update_interval_hours',
    'allowlist', 'custom_rules',
  ]);
});
