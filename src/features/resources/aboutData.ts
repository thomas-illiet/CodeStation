/** Collects and serializes the machine inventory shown by About this Code Station. */
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { InstalledRecord } from "../packages/types.js";

const execFileAsync = promisify(execFile);
const COMMAND_TIMEOUT_MS = 15_000;
const MAX_OUTPUT_BYTES = 16 * 1024 * 1024;

export type DevelopmentDependency = { id: "python" | "node" | "go"; name: string; path: string; version: string };
export type SystemPackage = { name: string; version: string; architecture: string };
export type PythonPackage = { name: string; version: string };
export type CollectionResult<T> = { collectedAt?: string; data: T; error?: string };
export type AboutSnapshot = {
  development: CollectionResult<DevelopmentDependency[]>;
  system: CollectionResult<SystemPackage[]>;
  python: CollectionResult<PythonPackage[]>;
  codestation: CollectionResult<InstalledRecord[]>;
};

type CommandResult = { stdout: string; stderr: string };
export type CommandRunner = (command: string, args: string[]) => Promise<CommandResult>;

export const runCommand: CommandRunner = async (command, args) => {
  const result = await execFileAsync(command, args, { timeout: COMMAND_TIMEOUT_MS, maxBuffer: MAX_OUTPUT_BYTES, encoding: "utf8" });
  return { stdout: result.stdout, stderr: result.stderr };
};

function message(error: unknown): string {
  if (error instanceof Error && error.message) return error.message.split("\n", 1)[0] ?? "Collection failed";
  return String(error);
}

async function executablePath(command: string, runner: CommandRunner): Promise<string> {
  const { stdout } = await runner("which", [command]);
  const resolved = stdout.trim().split("\n", 1)[0];
  if (!resolved) throw new Error(`${command} is not available`);
  return resolved;
}

function normalizeVersion(id: DevelopmentDependency["id"], output: string): string {
  const value = output.trim();
  if (id === "python") return value.replace(/^Python\s+/i, "");
  if (id === "node") return value.replace(/^v/, "");
  if (id === "go") return value.match(/\bgo([\w.-]+)/)?.[1] ?? value;
  return value;
}

const DEPENDENCIES = [
  { id: "python", name: "Python", commands: ["python3", "python"], args: ["--version"] },
  { id: "node", name: "Node.js", commands: ["node"], args: ["--version"] },
  { id: "go", name: "Go", commands: ["go"], args: ["version"] }
] as const;

/** Detects one development runtime without failing the other rows. */
export async function collectDevelopmentDependency(id: DevelopmentDependency["id"], runner: CommandRunner = runCommand): Promise<DevelopmentDependency> {
  const definition = DEPENDENCIES.find((item) => item.id === id);
  if (!definition) throw new Error(`Unknown development dependency: ${id}`);
  let command: string | undefined;
  let path: string | undefined;
  for (const candidate of definition.commands) {
    try { path = await executablePath(candidate, runner); command = candidate; break; } catch { /* Try the next supported executable name. */ }
  }
  if (!command || !path) throw new Error(`${definition.name} is not available`);
  const { stdout, stderr } = await runner(command, [...definition.args]);
  const version = normalizeVersion(id, stdout || stderr);
  if (!version) throw new Error(`${definition.name} did not return a version`);
  return { id, name: definition.name, path, version };
}

/** Parses the stable tab-separated dpkg-query output. */
export function parseSystemPackages(output: string): SystemPackage[] {
  return output.split("\n").filter(Boolean).map((line) => {
    const [name, version, architecture] = line.split("\t");
    if (!name || !version || !architecture) throw new Error("dpkg-query returned an invalid package row");
    return { name, version, architecture };
  }).sort((left, right) => left.name.localeCompare(right.name, "en"));
}

/** Collects every package registered by dpkg. */
export async function collectSystemPackages(runner: CommandRunner = runCommand): Promise<SystemPackage[]> {
  try {
    const { stdout } = await runner("dpkg-query", ["-W", "-f=${binary:Package}\\t${Version}\\t${Architecture}\\n"]);
    return parseSystemPackages(stdout);
  } catch {
    throw new Error("System package inventory is unavailable");
  }
}

/** Parses pip's machine-readable package list. */
export function parsePythonPackages(output: string): PythonPackage[] {
  let decoded: unknown;
  try { decoded = JSON.parse(output); } catch { throw new Error("pip returned invalid JSON"); }
  if (!Array.isArray(decoded)) throw new Error("pip returned an invalid package list");
  return decoded.map((item) => {
    if (!item || typeof item !== "object" || typeof (item as { name?: unknown }).name !== "string" || typeof (item as { version?: unknown }).version !== "string") {
      throw new Error("pip returned an invalid package entry");
    }
    return { name: (item as { name: string }).name, version: (item as { version: string }).version };
  }).sort((left, right) => left.name.localeCompare(right.name, "en"));
}

/** Collects packages for the same Python runtime displayed in the dependency table. */
export async function collectPythonPackages(runner: CommandRunner = runCommand): Promise<PythonPackage[]> {
  for (const command of ["python3", "python"]) {
    try {
      await executablePath(command, runner);
      const { stdout } = await runner(command, ["-m", "pip", "list", "--format=json", "--disable-pip-version-check"]);
      return parsePythonPackages(stdout);
    } catch { /* Fall back from python3 to python. */ }
  }
  throw new Error("Python package inventory is unavailable");
}

export function successful<T>(data: T): CollectionResult<T> { return { data, collectedAt: new Date().toISOString() }; }
export function failed<T>(previous: CollectionResult<T>, error: unknown): CollectionResult<T> { return { ...previous, error: message(error) }; }

/** Creates the privacy-bounded report persisted by the Export action. */
export function buildMachineReport(snapshot: AboutSnapshot, extensionVersion: string, editorVersion: string): object {
  const codestation = {
    ...snapshot.codestation,
    data: snapshot.codestation.data.map(({ namespace, name, version, status, installedAt, updatedAt }) => ({
      namespace, name, version, status, installedAt, updatedAt
    }))
  };
  return {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    machine: { os: process.platform, architecture: process.arch },
    software: { codestation: extensionVersion, editor: editorVersion },
    inventories: { ...snapshot, codestation }
  };
}
