/** Package feature composition and the package data boundary used by the sidebar. */
import * as vscode from "vscode";
import { CatalogCache } from "./catalogCache.js";
import { LocalRegistry } from "./localRegistry.js";
import { OperationManager } from "./operations.js";
import { RegistryClient } from "./registryClient.js";
import type { CatalogPage, InstalledRecord } from "./types.js";
import { PackagePanel, ProgressPanel } from "./webviews.js";

export type PackageSidebarMessage = {
  type?: unknown;
  action?: unknown;
  query?: unknown;
  category?: unknown;
  sort?: unknown;
  namespace?: unknown;
  name?: unknown;
};

/** Owns package data and operations while the single CodeStation sidebar owns rendering. */
export class PackageFeature implements vscode.Disposable {
  private readonly cache = new CatalogCache();
  private readonly client = new RegistryClient();
  private readonly local = new LocalRegistry();
  private readonly operations: OperationManager;
  private readonly operationSubscription: vscode.Disposable;
  private webview?: vscode.Webview;
  private query = "";
  private category = "";
  private sort = "name";

  constructor(private readonly context: vscode.ExtensionContext) {
    this.operations = new OperationManager(context.extensionPath, this.client, this.local);
    this.operationSubscription = this.operations.subscribe((event) => {
      if (event.phase === "succeeded" || event.phase === "failed") void this.load();
    });
  }

  attach(webview: vscode.Webview): void { this.webview = webview; }
  dispose(): void {
    this.operationSubscription.dispose();
    this.operations.dispose();
  }
  refresh(): void { void this.load(); }

  async handle(message: PackageSidebarMessage): Promise<boolean> {
    if (message.type !== "packages" || typeof message.action !== "string") return false;

    if (message.action === "load") {
      await this.load();
    } else if (message.action === "search") {
      this.query = typeof message.query === "string" ? message.query : "";
      await this.load();
    } else if (message.action === "filter") {
      this.category = typeof message.category === "string" ? message.category : "";
      this.sort = message.sort === "updated" ? "updated" : "name";
      await this.load();
    } else if (message.action === "open" && typeof message.namespace === "string" && typeof message.name === "string") {
      await PackagePanel.show(this.client, this.local, this.operations, this.context, message.namespace, message.name);
    } else if (["install", "update"].includes(message.action) && typeof message.namespace === "string" && typeof message.name === "string") {
      const manifest = await this.client.detail(message.namespace, message.name);
      const kind = message.action as "install" | "update";
      const operationId = this.operations.enqueue(kind, manifest);
      const icon = await this.client.dataUrl(`/v1/packages/${manifest.namespace}/${manifest.name}/${manifest.version}/icon`);
      ProgressPanel.show(this.operations, this.context, operationId, manifest, icon);
    }
    return true;
  }

  async publishUpdateCount(catalog?: CatalogPage, records?: InstalledRecord[]): Promise<number> {
    try {
      const [resolvedCatalog, resolvedRecords] = await Promise.all([catalog ?? this.client.list(), records ?? this.local.list()]);
      const installed = new Map(resolvedRecords.filter((record) => record.status === "installed").map((record) => [`${record.namespace}/${record.name}`, record]));
      const count = resolvedCatalog.items.filter((item) => {
        const record = installed.get(`${item.namespace}/${item.name}`);
        return record !== undefined && record.version !== item.latestVersion;
      }).length;
      await vscode.commands.executeCommand("codestation.setUpdateCount", count);
      return count;
    } catch {
      return 0;
    }
  }

  private post(value: object): void { void this.webview?.postMessage(value); }

  private async load(): Promise<void> {
    this.post({ type: "packagesLoading" });
    const records = await this.local.list();
    const installed = records.filter((record) => record.status === "installed");
    try {
      const page = await this.client.list(this.query, 1, this.category, this.sort);
      page.items = await Promise.all(page.items.map(async (item) => ({ ...item, iconUrl: await this.client.dataUrl(item.iconUrl) })));
      if (!this.query && !this.category) await this.cache.save(page);
      await this.publishUpdateCount(!this.query && !this.category ? page : undefined, records);
      this.postCatalog(page, installed, false);
    } catch {
      this.postCatalog(await this.offlinePage(installed), installed, true);
    }
  }

  private postCatalog(page: CatalogPage, installed: InstalledRecord[], offline: boolean): void {
    this.post({ type: "packagesCatalog", page, installed, category: this.category, sort: this.sort, offline, offlineMessage: offline ? "Unable to contact the package registry. Only installed packages are shown." : "" });
  }

  private async offlinePage(installed: InstalledRecord[]): Promise<CatalogPage> {
    const cached = await this.cache.load();
    const cachedItems = new Map((cached?.page.items ?? []).map((item) => [`${item.namespace}/${item.name}`, item]));
    let items = installed.flatMap((record) => {
      const item = cachedItems.get(`${record.namespace}/${record.name}`);
      return item ? [item] : [];
    });
    const term = this.query.trim().toLocaleLowerCase();
    if (term) items = items.filter((item) => `${item.namespace} ${item.name} ${item.summary} ${item.publisher}`.toLocaleLowerCase().includes(term));
    if (this.category) items = items.filter((item) => item.categories.includes(this.category));
    items.sort((left, right) => this.sort === "updated" ? right.updatedAt.localeCompare(left.updatedAt) : left.name.localeCompare(right.name));
    return { items, page: 1, pageSize: items.length, total: items.length, totalPages: 1 };
  }
}

/** Creates package services and registers package-related commands. */
export function activatePackages(context: vscode.ExtensionContext, openPackages: () => Promise<void>): PackageFeature {
  const feature = new PackageFeature(context);
  context.subscriptions.push(
    feature,
    vscode.commands.registerCommand("codestation.open", openPackages),
    vscode.commands.registerCommand("codestation.refresh", () => feature.refresh()),
    vscode.commands.registerCommand("codestation.showInstalled", async () => {
      await openPackages();
      void vscode.window.showInformationMessage("Use the Installed section to manage local packages.");
    }),
    vscode.commands.registerCommand("codestation.showOperations", () => {
      void vscode.window.showInformationMessage("Package operations remain available from their CodeStation operation page.");
    })
  );
  void feature.publishUpdateCount();
  return feature;
}
