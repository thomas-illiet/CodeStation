/** Canonical package storage paths and traversal-safe identifiers. */
import * as os from 'node:os';
import * as path from 'node:path';

function xdg(envName: string, fallback: string): string {
  return process.env[envName] || path.join(os.homedir(), fallback);
}

/** Filesystem locations owned by the current user's package subsystem. */
export const paths = {
  data: path.join(xdg('XDG_DATA_HOME', '.local/share'), 'codestation'),
  cache: path.join(xdg('XDG_CACHE_HOME', '.cache'), 'codestation'),
  state: path.join(xdg('XDG_STATE_HOME', '.local/state'), 'codestation'),
  package(namespace: string, name: string): string {
    return path.join(this.data, 'installations', safeSegment(namespace), safeSegment(name));
  },
  log(operationId: string): string { return path.join(this.state, 'logs', `${safeSegment(operationId)}.log`); },
  registry: (): string => path.join(paths.data, 'registry.json'),
  catalogCache: (): string => path.join(paths.cache, 'registry-catalog.json'),
  lock: (): string => path.join(paths.data, 'operation.lock')
};

/** Accepts only canonical identifiers safe for use as one path segment. */
export function safeSegment(value: string): string {
  if (!/^[a-z0-9][a-z0-9-]{1,62}$/.test(value)) throw new Error(`Invalid package identifier: ${value}`);
  return value;
}
