/** Manifest validation, signature verification, hashing and log redaction. */
import { createHash, createPublicKey, verify } from 'node:crypto';
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { PackageVersion } from './types';

/** Returns the lowercase SHA-256 digest of binary data. */
export function sha256(data: Uint8Array): string { return createHash('sha256').update(data).digest('hex'); }

/** Rejects malformed or unsafe manifest values before any network download. */
export function validateManifest(manifest: PackageVersion): void {
  const identifier = /^[a-z0-9][a-z0-9-]{1,62}$/;
  const digest = /^[a-f0-9]{64}$/;
  if (manifest.schemaVersion !== 1) throw new Error('Unsupported package manifest version');
  if (!identifier.test(manifest.namespace) || !identifier.test(manifest.name)) throw new Error('Invalid package identifier');
  if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$/.test(manifest.version)) throw new Error('Invalid package version');
  const platform = process.platform === 'win32' ? 'windows' : process.platform;
  const arch = process.arch === 'x64' ? 'amd64' : process.arch;
  const target = `${platform}-${arch}`;
  if (!manifest.targets?.includes(target as PackageVersion['targets'][number])) throw new Error(`Package does not support ${target}`);
  if (!digest.test(manifest.archive?.sha256 || '')) throw new Error('Invalid package archive digest');
  if (!Number.isSafeInteger(manifest.archive?.size) || manifest.archive.size! < 1 || manifest.archive.size! > 536_870_912) throw new Error('Invalid package archive size');
}

/** Verifies the manifest signature against a bundled trusted public key. */
export async function verifyManifest(manifest: PackageVersion, extensionRoot: string): Promise<void> {
  validateManifest(manifest);
  if (manifest.signature.algorithm !== 'ed25519') throw new Error('Unsupported manifest signature algorithm');
  const keyPath = path.join(extensionRoot, 'keys', `${manifest.signature.keyId}.pem`);
  let pem: string;
  try { pem = await fs.readFile(keyPath, 'utf8'); } catch { throw new Error(`Unknown signing key: ${manifest.signature.keyId}`); }
  const unsigned = { ...manifest, signature: undefined } as Record<string, unknown>;
  delete unsigned.signature;
  const canonical = Buffer.from(stableJson(unsigned));
  if (!verify(null, canonical, createPublicKey(pem), Buffer.from(manifest.signature.value, 'base64'))) throw new Error('Manifest signature is invalid');
}

/** Serializes JSON deterministically for signature verification. */
export function stableJson(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(stableJson).join(',')}]`;
  const object = value as Record<string, unknown>;
  return `{${Object.keys(object).sort().map(key => `${JSON.stringify(key)}:${stableJson(object[key])}`).join(',')}}`;
}

/** Removes common credential shapes before text reaches logs or UI. */
export function redact(line: string): string {
  return line
    .replace(/((?:token|password|secret|api[_-]?key)\s*[=:]\s*)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/Bearer\s+[A-Za-z0-9._~+\/-]+=*/gi, 'Bearer [REDACTED]');
}
