import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { collectDefinedKeys } from './check-missing-i18n.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const DICTIONARY = join(root, 'web', 'src', 'i18n', 'index.tsx');
const DATASET = join(root, 'deploy', 'cloudflare', 'data', 'responses.json');
const PREFIX = 'agent.capability.';

/**
 * The capability directory reads in the operator's language, so every registered capability needs
 * a title and a description in the dictionary. The console falls back to the registry's identifier
 * when one is missing, which keeps a rolling deployment readable but would let a new capability
 * ship untranslated without anyone noticing - hence this check.
 *
 * The registry is read from the demonstration's dataset because that is generated from the real
 * handlers and kept fresh by its own gate, so it is the registry as the server builds it.
 */
function registeredCapabilities() {
  const dataset = JSON.parse(readFileSync(DATASET, 'utf8'));
  const body = JSON.parse(dataset.responses.capabilities.body);
  return body.capabilities.map((capability) => capability.name);
}

test('every registered capability has a localized title and description', () => {
  const keys = collectDefinedKeys(DICTIONARY);
  const names = registeredCapabilities();
  assert.ok(names.length > 0, 'the dataset lists no capabilities');
  const missing = names.flatMap((name) => [`${PREFIX}${name}`, `${PREFIX}${name}.description`].filter((key) => !keys.has(key)));
  assert.deepEqual(missing, []);
});

test('the dictionary names no capability the registry lacks', () => {
  const names = new Set(registeredCapabilities());
  const stale = [...collectDefinedKeys(DICTIONARY)]
    .filter((key) => key.startsWith(PREFIX))
    .map((key) => key.slice(PREFIX.length).replace(/\.description$/, ''))
    .filter((name) => !names.has(name));
  assert.deepEqual([...new Set(stale)], []);
});
