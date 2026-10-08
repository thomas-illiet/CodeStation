/** CodeStation extension composition root. */
import * as vscode from "vscode";
import { HomeProvider } from "./features/home/provider.js";
import { applyStoredProxy, clearHttpProxy, openProxyModal, testHttpProxy } from "./features/network/proxy.js";
import { openPortTest } from "./features/network/portTest.js";
import { activatePackages } from "./features/packages/index.js";
import { openAboutPanel } from "./features/resources/aboutPanel.js";
import { registerResourceStatusBar } from "./features/resources/statusBar.js";
import { reloadVsCodeWindow, restartCodeServer } from "./features/system/serverControl.js";
import { showWorkspaceStorageUsage } from "./features/workspace/storageUsage.js";

/** Activates every CodeStation domain and registers its VS Code contributions. */
export function activate(context: vscode.ExtensionContext): void {
  let home: HomeProvider;
  const packages = activatePackages(context, () => home.showSection("packages"));
  home = new HomeProvider(context, vscode.Uri.joinPath(context.extensionUri, "docs"), packages);
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("codestation.home", home, { webviewOptions: { retainContextWhenHidden: true } }),
    vscode.commands.registerCommand("codestation.back", () => home.showSection("home")),
    vscode.commands.registerCommand("codestation.setUpdateCount", (count: number) => home.setUpdateCount(count)),
    vscode.commands.registerCommand("codestation.openAbout", () => openAboutPanel(context)),
    vscode.commands.registerCommand("codestation.configureHttpProxy", () => openProxyModal(context)),
    vscode.commands.registerCommand("codestation.clearHttpProxy", () => clearHttpProxy(context)),
    vscode.commands.registerCommand("codestation.testHttpProxy", () => testHttpProxy(context)),
    vscode.commands.registerCommand("codestation.testPort", () => openPortTest(context)),
    vscode.commands.registerCommand("codestation.workspaceStorageUsage", () => showWorkspaceStorageUsage(context)),
    vscode.commands.registerCommand("codestation.reloadWindow", reloadVsCodeWindow),
    vscode.commands.registerCommand("codestation.restartCodeServer", restartCodeServer)
  );

  registerResourceStatusBar(context);
  void applyStoredProxy(context);
}

/** VS Code disposes registered subscriptions through the extension context. */
export function deactivate(): void {}
