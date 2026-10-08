/** Portable cgroup v2/v1 resource readers and pure parsing helpers. */
import { readFile } from "node:fs/promises";
import os from "node:os";

/** CPU quota and cumulative usage sampled from cgroups or the host. */
export interface CpuSnapshot {
  usageMicros: number;
  timestampMicros: number;
  limitCores: number;
}

/** Memory usage and effective limit sampled from cgroups or the host. */
export interface MemorySnapshot {
  usedBytes: number;
  limitBytes: number;
}

async function firstReadable(paths: string[]): Promise<string | undefined> {
  for (const path of paths) {
    try {
      return (await readFile(path, "utf8")).trim();
    } catch {
      // Try cgroup v1 after v2, then fall back to host metrics.
    }
  }
  return undefined;
}

/** Converts a cgroup v2 `cpu.max` value into an effective core limit. */
export function parseCpuMax(value: string | undefined, fallbackCores: number): number {
  if (!value) return fallbackCores;
  const [quotaText, periodText] = value.trim().split(/\s+/);
  if (!quotaText || quotaText === "max") return fallbackCores;
  const quota = Number(quotaText);
  const period = Number(periodText);
  return quota > 0 && period > 0 ? quota / period : fallbackCores;
}

/** Extracts cumulative microseconds from a cgroup v2 `cpu.stat` payload. */
export function parseCpuStat(value: string | undefined): number | undefined {
  const match = value?.match(/^usage_usec\s+(\d+)$/m);
  return match?.[1] ? Number(match[1]) : undefined;
}

/** Replaces missing or unlimited cgroup limits with the host total. */
export function normalizeMemoryLimit(raw: string | undefined, hostTotal: number): number {
  if (!raw || raw === "max") return hostTotal;
  const parsed = Number(raw);
  // cgroup v1 represents "unlimited" with a huge integer near LONG_MAX.
  return Number.isFinite(parsed) && parsed > 0 && parsed < hostTotal * 100
    ? parsed
    : hostTotal;
}

/** Reads memory accounting with cgroup v2, v1 and host fallbacks. */
export async function readMemorySnapshot(): Promise<MemorySnapshot> {
  const hostTotal = os.totalmem();
  const [currentRaw, limitRaw] = await Promise.all([
    firstReadable([
      "/sys/fs/cgroup/memory.current",
      "/sys/fs/cgroup/memory/memory.usage_in_bytes"
    ]),
    firstReadable([
      "/sys/fs/cgroup/memory.max",
      "/sys/fs/cgroup/memory/memory.limit_in_bytes"
    ])
  ]);
  const limitBytes = normalizeMemoryLimit(limitRaw, hostTotal);
  const cgroupUsed = Number(currentRaw);
  const usedBytes = Number.isFinite(cgroupUsed)
    ? Math.min(Math.max(0, cgroupUsed), limitBytes)
    : hostTotal - os.freemem();
  return { usedBytes, limitBytes };
}

/** Reads CPU accounting with cgroup v2, v1 and host fallbacks. */
export async function readCpuSnapshot(): Promise<CpuSnapshot> {
  const fallbackCores = Math.max(1, os.cpus().length);
  const [cpuMax, cpuStat, v1Quota, v1Period, v1Usage] = await Promise.all([
    firstReadable(["/sys/fs/cgroup/cpu.max"]),
    firstReadable(["/sys/fs/cgroup/cpu.stat"]),
    firstReadable(["/sys/fs/cgroup/cpu/cpu.cfs_quota_us"]),
    firstReadable(["/sys/fs/cgroup/cpu/cpu.cfs_period_us"]),
    firstReadable(["/sys/fs/cgroup/cpuacct/cpuacct.usage"])
  ]);

  const v1Limit = v1Quota && v1Period && Number(v1Quota) > 0
    ? Number(v1Quota) / Number(v1Period)
    : fallbackCores;
  const limitCores = cpuMax ? parseCpuMax(cpuMax, fallbackCores) : v1Limit;
  const v2Usage = parseCpuStat(cpuStat);
  const usageMicros = v2Usage ?? (v1Usage ? Number(v1Usage) / 1_000 : 0);
  return {
    usageMicros: Number.isFinite(usageMicros) ? usageMicros : 0,
    timestampMicros: Number(process.hrtime.bigint() / 1_000n),
    limitCores
  };
}

/** Calculates quota-relative CPU usage between two cumulative samples. */
export function cpuUsagePercent(previous: CpuSnapshot | undefined, current: CpuSnapshot): number | undefined {
  if (!previous) return undefined;
  const elapsed = current.timestampMicros - previous.timestampMicros;
  const capacity = elapsed * current.limitCores;
  if (elapsed <= 0 || capacity <= 0) return undefined;
  return Math.min(100, Math.max(0, ((current.usageMicros - previous.usageMicros) / capacity) * 100));
}
