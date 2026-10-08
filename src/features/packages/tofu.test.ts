import assert from 'node:assert/strict';
import test from 'node:test';
import { validateRegistryUrl } from './tofu';

test('accepts HTTP and HTTPS registries', () => {
  assert.equal(validateRegistryUrl('http://registry.internal:8080').protocol, 'http:');
  assert.equal(validateRegistryUrl('https://registry.internal').protocol, 'https:');
});

test('rejects non-HTTP registry protocols', () => {
  assert.throws(
    () => validateRegistryUrl('file:///tmp/registry'),
    /requires an HTTP or HTTPS registry/,
  );
});
