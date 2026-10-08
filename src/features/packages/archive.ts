/** Safely extracts package tarballs through a bounded child process. */
import { spawn } from 'node:child_process';
import { promises as fs } from 'node:fs';
import * as path from 'node:path';

/** Extracts a gzip tar archive after rejecting unsafe members. */
export async function extractTarGz(archive: string, destination: string): Promise<void> {
  const verbose = await command('tar', ['-tvzf', archive]);
  for (const line of verbose.split('\n').filter(Boolean)) {
    const type = line[0];
    if (type !== '-' && type !== 'd') throw new Error('Package archives may contain only regular files and directories');
  }
  const listing = await command('tar', ['-tzf', archive]);
  for (const name of listing.split('\n').filter(Boolean)) {
    const normalized = path.posix.normalize(name);
    if (path.posix.isAbsolute(name) || normalized === '..' || normalized.startsWith('../')) throw new Error(`Unsafe archive path: ${name}`);
  }
  await fs.mkdir(destination, { recursive: true, mode: 0o700 });
  await command('tar', ['-xzf', archive, '--no-same-owner', '--no-same-permissions', '-C', destination]);
}

function command(executable: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => { let stdout='';let stderr='';const child=spawn(executable,args,{stdio:['ignore','pipe','pipe']});child.stdout.on('data',d=>stdout+=d);child.stderr.on('data',d=>stderr+=d);child.on('error',reject);child.on('close',code=>code===0?resolve(stdout):reject(new Error(`${executable} failed: ${stderr}`))); });
}
