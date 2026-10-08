/** Unit tests for manifest validation, canonical JSON and redaction. */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { redact, stableJson, validateManifest } from './security.js';
import { PackageVersion } from './types.js';

test('stableJson canonicalizes object keys recursively', () => {
  assert.equal(stableJson({ z: 1, a: { d: 2, b: 3 } }), '{"a":{"b":3,"d":2},"z":1}');
});

test('redact removes common credential shapes', () => {
  assert.equal(redact('token=abc123 password: hunter2'), 'token=[REDACTED] password: [REDACTED]');
  assert.equal(redact('Authorization: Bearer abc.def.ghi'), 'Authorization: Bearer [REDACTED]');
});

test('validateManifest rejects unbounded archives before download', () => {
  const platform = process.platform === 'win32' ? 'windows' : process.platform;
  const arch = process.arch === 'x64' ? 'amd64' : process.arch;
  const manifest = { schemaVersion: 1, namespace: 'core', name: 'demo', version: '1.0.0', platforms: [platform], architectures: [arch], targets: [`${platform}-${arch}`], archive: { path: 'x', sha256: 'a'.repeat(64), size: 536_870_913 } } as PackageVersion;
  assert.throws(() => validateManifest(manifest), /archive size/);
});
