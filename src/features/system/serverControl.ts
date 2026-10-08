/** Controlled code-server lifecycle actions. */
import * as vscode from "vscode";

/** Requests a container-safe code-server restart after explicit confirmation. */
export async function restartCodeServer(): Promise<boolean> {
  const choice = await vscode.window.showWarningMessage(
    "Restart code-server now? Open editors and terminals will reconnect automatically, but unsaved terminal input may be lost.",
    { modal: true },
    "Restart"
  );
  if (choice !== "Restart") return false;

  void vscode.window.showInformationMessage("Code-server is restarting…");
  setTimeout(() => {
    try {
      process.kill(1, "SIGTERM");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      void vscode.window.showErrorMessage(`Unable to restart code-server: ${message}`);
    }
  }, 750);
  return true;
}

/** Reloads the current VS Code window using the native workbench action. */
export async function reloadVsCodeWindow(): Promise<void> {
  await vscode.commands.executeCommand("workbench.action.reloadWindow");
}
