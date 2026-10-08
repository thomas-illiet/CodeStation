/** Executes the bundled OpenTofu CLI and translates JSON output to events. */
import { spawn } from 'node:child_process';
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { createInterface } from 'node:readline';
import { OperationEvent } from './types';
import { redact } from './security';

/** Runs the platform-specific embedded OpenTofu binary without a shell. */
export class TofuRunner {
  constructor(private readonly extensionRoot: string, private readonly registryUrl: string) {}

  binary(): string {
    const platform = process.platform === 'linux' ? 'linux' : process.platform === 'win32' ? 'windows' : undefined;
    if (!platform) throw new Error(`Unsupported platform: ${process.platform}`);
    const arch = process.arch === 'x64' ? 'x64' : process.arch === 'arm64' ? 'arm64' : undefined;
    if (!arch) throw new Error(`Unsupported architecture: ${process.arch}`);
    if (platform === 'windows' && arch === 'arm64') throw new Error('Unsupported platform: windows/arm64');
    return path.join(this.extensionRoot, 'bin', `${platform}-${arch}`, platform === 'windows' ? 'tofu.exe' : 'tofu');
  }

  async prepareConfig(workDir: string): Promise<string> {
    const binary = this.binary(); await fs.access(binary, process.platform === 'win32' ? fs.constants.F_OK : fs.constants.X_OK);
    await fs.mkdir(workDir, { recursive: true, mode: 0o700 });
    validateRegistryUrl(this.registryUrl);
    const providerMirror = path.join(this.extensionRoot, 'providers').replace(/\\/g, '/');
    await fs.access(providerMirror, fs.constants.R_OK);
    const config = `provider_installation {\n  filesystem_mirror {\n    path = "${providerMirror}"\n    include = ["codestation.internal/codestation/packages"]\n  }\n}\n`;
    const file = path.join(workDir, '.tofurc'); await fs.writeFile(file, config, { mode: 0o600 }); return file;
  }

  async run(args: string[], workDir: string, operationId: string, phase: OperationEvent['phase'], onEvent: (event: OperationEvent) => void): Promise<void> {
    const config = await this.prepareConfig(workDir);
    await new Promise<void>((resolve, reject) => {
      const child = spawn(this.binary(), args, { cwd: workDir, env: { ...process.env, TF_CLI_CONFIG_FILE: config, TF_IN_AUTOMATION: '1', CHECKPOINT_DISABLE: '1' }, stdio: ['ignore', 'pipe', 'pipe'] });
      const emit = (raw: string) => onEvent({ operationId, phase, message: parseMessage(raw), raw: redact(raw), timestamp: new Date().toISOString() });
      createInterface({ input: child.stdout }).on('line', emit); createInterface({ input: child.stderr }).on('line', emit);
      child.on('error', reject); child.on('close', code => code === 0 ? resolve() : reject(new Error(`OpenTofu exited with code ${code}`)));
    });
  }
}

export function validateRegistryUrl(value: string): URL {
  const registry = new URL(value);
  if (!['http:', 'https:'].includes(registry.protocol)) throw new Error('Package installation requires an HTTP or HTTPS registry');
  return registry;
}

function parseMessage(line: string): string {
  try { const parsed = JSON.parse(line) as { '@message'?: string; diagnostic?: { summary?: string } }; return redact(parsed['@message'] || parsed.diagnostic?.summary || line); }
  catch { return redact(line); }
}
