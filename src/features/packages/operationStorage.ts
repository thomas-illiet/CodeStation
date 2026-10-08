/** Filesystem safeguards and audit-log persistence for package operations. */
import { promises as fs } from "node:fs";
import * as path from "node:path";
import * as vscode from "vscode";
import { paths } from "./paths.js";
import { processStartId } from "./processIdentity.js";
import { redact } from "./security.js";
import type { OperationEvent } from "./types.js";

type OperationLock = {
  pid: number;
  startedAt: string;
  processStartId?: string;
};

/** Creates protected state directories and applies log retention. */
export async function ensureOperationDirectories(): Promise<void> {
  for (const directory of [paths.data, paths.cache, path.join(paths.state, "logs")]) {
    await fs.mkdir(directory, { recursive: true, mode: 0o700 });
    await fs.chmod(directory, 0o700);
  }
  await rotateLogs();
}

/** Creates a private recovery copy of an existing OpenTofu state. */
export async function backupState(workDir: string): Promise<void> {
  const source = path.join(workDir, "config", "terraform.tfstate");
  try {
    await fs.copyFile(source, `${source}.codestation-backup`);
    await fs.chmod(`${source}.codestation-backup`, 0o600);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
}

/** Restricts generated plans and state files to their workspace owner. */
export async function protectWorkspace(workDir: string): Promise<void> {
  const files = [
    path.join(workDir, "codestation.tfplan"),
    ...["terraform.tfstate", "terraform.tfstate.backup", "terraform.tfstate.codestation-backup"]
      .map((name) => path.join(workDir, "config", name))
  ];
  for (const file of files) {
    try { await fs.chmod(file, 0o600); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error; }
  }
}

/** Appends a redacted event while enforcing the configured file-size limit. */
export async function appendOperationLog(event: OperationEvent): Promise<void> {
  const line = `${JSON.stringify({ ...event, raw: event.raw ? redact(event.raw) : undefined })}\n`;
  const file = paths.log(event.operationId);
  await fs.mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const max = vscode.workspace.getConfiguration("codestation").get<number>("maxLogSizeMiB", 20) * 1024 * 1024;
  try {
    if ((await fs.stat(file)).size + Buffer.byteLength(line) > max) await fs.rename(file, `${file}.1`);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
  await fs.appendFile(file, line, { mode: 0o600 });
}

/** Removes a lock when its owner exited or its PID was reused. */
export async function removeStaleLock(): Promise<boolean> {
  try {
    const lock = JSON.parse(await fs.readFile(paths.lock(), "utf8")) as OperationLock;
    try { process.kill(lock.pid, 0); }
    catch (error) {
      if ((error as NodeJS.ErrnoException).code === "EPERM") return false;
      await fs.unlink(paths.lock());
      return true;
    }

    const liveStartId = await processStartId(lock.pid);
    if (lock.processStartId && liveStartId && lock.processStartId !== liveStartId) {
      await fs.unlink(paths.lock());
      return true;
    }

    // Legacy locks did not record /proc identity. A matching PID can belong to
    // the freshly restarted extension host, so compare its actual start time.
    const currentProcessStartedAt = Date.now() - process.uptime() * 1000;
    if (lock.pid === process.pid && Date.parse(lock.startedAt) < currentProcessStartedAt - 1000) {
      await fs.unlink(paths.lock());
      return true;
    }
    return false;
  } catch { return false; }
}

async function rotateLogs(): Promise<void> {
  const days = vscode.workspace.getConfiguration("codestation").get<number>("logRetentionDays", 30);
  const cutoff = Date.now() - days * 86_400_000;
  const directory = path.join(paths.state, "logs");
  for (const file of await fs.readdir(directory)) {
    const full = path.join(directory, file);
    if ((await fs.stat(full)).mtimeMs < cutoff) await fs.unlink(full);
  }
}
