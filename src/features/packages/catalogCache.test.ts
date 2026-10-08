import { strict as assert } from 'node:assert';
import { promises as fs } from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { test } from 'node:test';
import { CatalogCache } from './catalogCache.js';
import type { CatalogPage } from './types.js';

const page: CatalogPage = {
  items: [{ namespace: 'core', name: 'hello', summary: 'Hello', publisher: 'CodeStation', categories: ['Tools'], latestVersion: '1.0.0', iconUrl: 'data:image/svg+xml;base64,AA==', updatedAt: '2026-10-04T00:00:00Z' }],
  page: 1, pageSize: 1, total: 1, totalPages: 1
};

test('catalog cache persists and reloads the last complete catalog', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'codestation-catalog-'));
  const cache = new CatalogCache(path.join(directory, 'catalog.json'));
  await cache.save(page);
  assert.deepEqual((await cache.load())?.page, page);
});

test('catalog cache ignores missing and malformed cache files', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'codestation-catalog-'));
  const file = path.join(directory, 'catalog.json');
  const cache = new CatalogCache(file);
  assert.equal(await cache.load(), undefined);
  await fs.writeFile(file, '{broken');
  assert.equal(await cache.load(), undefined);
});
