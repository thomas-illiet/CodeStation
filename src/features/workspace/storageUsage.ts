/** Read-only workspace storage inspection. */
import { lstat, opendir } from "node:fs/promises";
import * as path from "node:path";
import * as vscode from "vscode";
import { documentHtml, showCodestationPanel } from "../../core/webviewPanel.js";
import { formatBytes } from "./storageFormat.js";

interface StorageEntry { name: string; bytes: number; files: number; inaccessible: number }

function escapeHtml(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

async function measure(target: string, token: vscode.CancellationToken): Promise<Omit<StorageEntry, "name">> {
  if (token.isCancellationRequested) throw new vscode.CancellationError();
  try {
    const info = await lstat(target);
    if (info.isSymbolicLink()) return { bytes: 0, files: 0, inaccessible: 0 };
    if (!info.isDirectory()) return { bytes: info.size, files: 1, inaccessible: 0 };

    let bytes = 0; let files = 0; let inaccessible = 0;
    const directory = await opendir(target);
    for await (const child of directory) {
      const usage = await measure(path.join(target, child.name), token);
      bytes += usage.bytes; files += usage.files; inaccessible += usage.inaccessible;
    }
    return { bytes, files, inaccessible };
  } catch (error) {
    if (error instanceof vscode.CancellationError) throw error;
    return { bytes: 0, files: 0, inaccessible: 1 };
  }
}

async function inspectFolder(folder: vscode.WorkspaceFolder, token: vscode.CancellationToken): Promise<StorageEntry[]> {
  if (folder.uri.scheme !== "file") return [];
  const directory = await opendir(folder.uri.fsPath);
  const entries: StorageEntry[] = [];
  for await (const child of directory) {
    if (token.isCancellationRequested) throw new vscode.CancellationError();
    const usage = await measure(path.join(folder.uri.fsPath, child.name), token);
    entries.push({ name: child.name, ...usage });
  }
  return entries.sort((left, right) => right.bytes - left.bytes);
}

/** Measures top-level workspace entries and renders the result in the shared CodeStation panel. */
export async function showWorkspaceStorageUsage(context: vscode.ExtensionContext): Promise<void> {
  const folders = vscode.workspace.workspaceFolders ?? [];
  if (!folders.length) { void vscode.window.showWarningMessage("Open a workspace before inspecting its storage usage."); return; }

  const results = await vscode.window.withProgress({
    location: vscode.ProgressLocation.Notification,
    title: "CodeStation: measuring workspace storage usage",
    cancellable: true
  }, async (_progress, token) => Promise.all(folders.map(async (folder) => ({ folder, entries: await inspectFolder(folder, token) }))));

  const sections = results.map(({ folder, entries }) => {
    const total = entries.reduce((sum, entry) => sum + entry.bytes, 0);
    const inaccessible = entries.reduce((sum, entry) => sum + entry.inaccessible, 0);
    const rows = entries.slice(0, 50).map((entry) => `<tr><td>${escapeHtml(entry.name)}</td><td>${formatBytes(entry.bytes)}</td><td>${entry.files.toLocaleString()}</td></tr>`).join("");
    return `<section><h2>${escapeHtml(folder.name)}</h2><p class="summary">${formatBytes(total)} across ${entries.length.toLocaleString()} top-level entries${inaccessible ? ` · ${inaccessible.toLocaleString()} inaccessible paths` : ""}</p><table><thead><tr><th>Entry</th><th>Size</th><th>Files</th></tr></thead><tbody>${rows}</tbody></table>${entries.length > 50 ? `<p class="hint">Showing the 50 largest entries.</p>` : ""}</section>`;
  }).join("");
  const nonce = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const body = `<h1>Workspace storage usage</h1><p class="intro">Read-only measurement. Symbolic links are not followed.</p>${sections}`;
  const extraStyle = `section{margin:20px 0}.summary,.hint{color:var(--vscode-descriptionForeground)}table{width:100%;border-collapse:collapse}th,td{padding:7px 8px;border-bottom:1px solid var(--vscode-panel-border);text-align:left}th:nth-child(2),th:nth-child(3),td:nth-child(2),td:nth-child(3){text-align:right;white-space:nowrap}`;
  showCodestationPanel(context, "Workspace storage usage", documentHtml("Workspace storage usage", body, nonce, "", extraStyle));
}
