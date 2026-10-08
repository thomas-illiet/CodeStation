/** Durable cache of the last complete package catalog received from the registry. */
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { paths } from './paths.js';
import type { CatalogPage } from './types.js';

interface CachedCatalog {
  savedAt: string;
  page: CatalogPage;
}

/** Reads and atomically replaces the registry catalog cache. */
export class CatalogCache {
  constructor(private readonly file = paths.catalogCache()) {}

  async load(): Promise<CachedCatalog | undefined> {
    try {
      const value = JSON.parse(await fs.readFile(this.file, 'utf8')) as CachedCatalog;
      if (!value || typeof value.savedAt !== 'string' || !Array.isArray(value.page?.items)) return undefined;
      return value;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT' || error instanceof SyntaxError) return undefined;
      throw error;
    }
  }

  async save(page: CatalogPage): Promise<void> {
    await fs.mkdir(path.dirname(this.file), { recursive: true, mode: 0o700 });
    const temporary = `${this.file}.${process.pid}.tmp`;
    await fs.writeFile(temporary, JSON.stringify({ savedAt: new Date().toISOString(), page }, null, 2), { mode: 0o600 });
    await fs.rename(temporary, this.file);
    await fs.chmod(this.file, 0o600);
  }
}
