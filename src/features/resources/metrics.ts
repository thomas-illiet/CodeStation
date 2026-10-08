/** Aggregates CPU, memory and filesystem measurements for presentation. */
import { statfs } from "node:fs/promises";
import { cpuUsagePercent, readCpuSnapshot, readMemorySnapshot, type CpuSnapshot } from "./cgroup.js";

/** Normalized resource snapshot consumed by all resource UI surfaces. */
export interface ResourceMetrics {
  cpuLimitCores: number;
  cpuUsedPercent?: number;
  memoryUsedBytes: number;
  memoryLimitBytes: number;
  diskUsedBytes: number;
  diskTotalBytes: number;
  diskPath: string;
}

/** Stateful reader that derives CPU usage between consecutive samples. */
export class MetricsReader {
  private previousCpu: CpuSnapshot | undefined;
  private diskCache?: { path: string; usedBytes: number; totalBytes: number; sampledAt: number };

  async read(diskPath: string): Promise<ResourceMetrics> {
    const diskPromise = this.diskCache && this.diskCache.path === diskPath && Date.now() - this.diskCache.sampledAt < 60_000
      ? Promise.resolve(this.diskCache)
      : statfs(diskPath).then((disk) => {
        const totalBytes = disk.blocks * disk.bsize;
        const cached = { path: diskPath, usedBytes: totalBytes - (disk.bavail * disk.bsize), totalBytes, sampledAt: Date.now() };
        this.diskCache = cached;
        return cached;
      });
    const [cpu, memory, disk] = await Promise.all([
      readCpuSnapshot(),
      readMemorySnapshot(),
      diskPromise
    ]);
    const cpuUsedPercent = cpuUsagePercent(this.previousCpu, cpu);
    this.previousCpu = cpu;
    return {
      cpuLimitCores: cpu.limitCores,
      cpuUsedPercent,
      memoryUsedBytes: memory.usedBytes,
      memoryLimitBytes: memory.limitBytes,
      diskUsedBytes: disk.usedBytes,
      diskTotalBytes: disk.totalBytes,
      diskPath
    };
  }
}
