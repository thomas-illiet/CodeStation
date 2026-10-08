/** Owns the container-resource status bar lifecycle. */
import * as vscode from "vscode";
import { MetricsReader, type ResourceMetrics } from "./metrics.js";

const UNITS = ["o", "Kio", "Mio", "Gio", "Tio"];

/** Formats a byte count with binary units for compact user-facing labels. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 o";
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNITS.length - 1);
  const value = bytes / 1024 ** index;
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${UNITS[index]}`;
}

function formatGib(bytes: number): string {
  return (Math.max(0, bytes) / 1024 ** 3).toFixed(1).replace(/\.0$/, "");
}

function resourceTooltip(metrics: ResourceMetrics): vscode.MarkdownString {
  const usedCores = metrics.cpuUsedPercent === undefined ? undefined : metrics.cpuLimitCores * metrics.cpuUsedPercent / 100;
  const cpu = usedCores === undefined ? "sampling in progress" : `${usedCores.toFixed(2)} cores used / ${metrics.cpuLimitCores.toFixed(2)} allocated`;
  const memoryFree = Math.max(0, metrics.memoryLimitBytes - metrics.memoryUsedBytes);
  const diskFree = Math.max(0, metrics.diskTotalBytes - metrics.diskUsedBytes);
  return new vscode.MarkdownString([
    "### Container resources",
    `- **CPU**: ${cpu}`,
    `- **Memory**: ${formatBytes(metrics.memoryUsedBytes)} used, ${formatBytes(memoryFree)} remaining, ${formatBytes(metrics.memoryLimitBytes)} limit`,
    `- **Disk ${metrics.diskPath}**: ${formatBytes(metrics.diskUsedBytes)} used, ${formatBytes(diskFree)} remaining, ${formatBytes(metrics.diskTotalBytes)} total`,
    "",
    `Last updated: ${new Date().toLocaleTimeString("en-US")}`
  ].join("\n"));
}

/** Registers status items, refresh commands and focus-aware sampling. */
export function registerResourceStatusBar(context: vscode.ExtensionContext): void {
  const cpu = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 12);
  const memory = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 11);
  const disk = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 10);
  const items = [cpu, memory, disk];
  cpu.name = "Container CPU used and allocated";
  memory.name = "Container RAM used and total";
  disk.name = "Container disk used and total";
  for (const item of items) item.command = "containerResourceMonitor.refresh";
  cpu.text = "$(chip) … CPU";
  memory.text = "$(circuit-board) …";
  disk.text = "$(database) …";
  items.forEach((item) => item.show());

  const reader = new MetricsReader();
  let timer: NodeJS.Timeout | undefined;
  let reading = false;
  const clearTimer = (): void => { if (timer) clearTimeout(timer); timer = undefined; };
  const refresh = async (): Promise<void> => {
    if (reading) return;
    reading = true;
    try {
      const config = vscode.workspace.getConfiguration("containerResourceMonitor");
      const metrics = await reader.read(config.get<string>("diskPath", "/home/coder"));
      const memoryFree = Math.max(0, metrics.memoryLimitBytes - metrics.memoryUsedBytes);
      const remainingPercent = metrics.memoryLimitBytes > 0 ? memoryFree / metrics.memoryLimitBytes * 100 : 100;
      const usedCores = metrics.cpuUsedPercent === undefined ? undefined : metrics.cpuLimitCores * metrics.cpuUsedPercent / 100;
      cpu.text = `$(chip) ${usedCores === undefined ? "…" : usedCores.toFixed(1)}/${metrics.cpuLimitCores.toFixed(1)} CPU`;
      memory.text = `$(circuit-board) ${formatGib(metrics.memoryUsedBytes)}/${formatGib(metrics.memoryLimitBytes)}G`;
      disk.text = `$(database) ${formatGib(metrics.diskUsedBytes)}/${formatGib(metrics.diskTotalBytes)}G`;
      const details = resourceTooltip(metrics);
      items.forEach((item) => { item.tooltip = details; item.show(); });
      cpu.backgroundColor = undefined;
      memory.backgroundColor = remainingPercent <= config.get<number>("memoryWarningRemainingPercent", 10)
        ? new vscode.ThemeColor("statusBarItem.errorBackground") : undefined;
    } catch (error) {
      cpu.text = "$(warning) Resources unavailable";
      cpu.tooltip = `Unable to read resource metrics: ${error instanceof Error ? error.message : String(error)}`;
      cpu.backgroundColor = new vscode.ThemeColor("statusBarItem.warningBackground");
      memory.hide();
      disk.hide();
    } finally { reading = false; }
  };
  const schedule = (): void => {
    clearTimer();
    if (!vscode.window.state.focused) return;
    const seconds = vscode.workspace.getConfiguration("containerResourceMonitor").get<number>("refreshIntervalSeconds", 10);
    timer = setTimeout(async () => { await refresh(); schedule(); }, Math.max(5, seconds) * 1_000);
  };

  context.subscriptions.push(
    ...items,
    vscode.commands.registerCommand("containerResourceMonitor.refresh", refresh),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("containerResourceMonitor")) { schedule(); void refresh(); }
    }),
    vscode.window.onDidChangeWindowState((state) => {
      if (!state.focused) { clearTimer(); return; }
      void refresh().finally(schedule);
    }),
    { dispose: clearTimer }
  );
  schedule();
  void refresh();
}
