/** Durable local inventory for packages managed by CodeStation. */
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { InstalledRecord } from './types';
import { paths } from './paths';

/** Reads and atomically updates the installed-package inventory. */
export class LocalRegistry {
  async list(): Promise<InstalledRecord[]> {
    try { return JSON.parse(await fs.readFile(paths.registry(), 'utf8')) as InstalledRecord[]; }
    catch (error: unknown) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []; throw error; }
  }

  async get(namespace: string, name: string): Promise<InstalledRecord | undefined> {
    return (await this.list()).find(item => item.namespace === namespace && item.name === name);
  }

  async save(record: InstalledRecord): Promise<void> {
    const records = (await this.list()).filter(item => item.namespace !== record.namespace || item.name !== record.name);
    records.push(record); await this.write(records);
  }

  async remove(namespace: string, name: string): Promise<void> {
    await this.write((await this.list()).filter(item => item.namespace !== namespace || item.name !== name));
  }

  async recoverInterrupted(): Promise<number> {
    const records = await this.list(); let recovered = 0;
    const updated = records.map(record => {
      if (!['installing', 'updating', 'uninstalling'].includes(record.status)) return record;
      recovered++;
      return { ...record, status: 'error' as const, updatedAt: new Date().toISOString(), lastError: 'The previous operation was interrupted when the VS Code extension host stopped. Retry from the package page.' };
    });
    if (recovered) await this.write(updated); return recovered;
  }

  private async write(records: InstalledRecord[]): Promise<void> {
    await fs.mkdir(path.dirname(paths.registry()), { recursive: true, mode: 0o700 });
    const temporary = `${paths.registry()}.${process.pid}.tmp`;
    await fs.writeFile(temporary, JSON.stringify(records, null, 2), { mode: 0o600 });
    await fs.rename(temporary, paths.registry());
    await fs.chmod(paths.registry(), 0o600);
  }
}
