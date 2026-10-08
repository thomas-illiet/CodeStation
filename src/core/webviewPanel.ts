/** Shared primitives for secure, singleton CodeStation webview panels. */
import * as vscode from "vscode";

let panel: vscode.WebviewPanel | undefined;
let currentMessageHandler: ((message: unknown) => void | Promise<void>) | undefined;

/** Opens or reuses the single CodeStation editor panel. */
export function showCodestationPanel(
  context: vscode.ExtensionContext,
  title: string,
  html: string,
  messageHandler?: (message: unknown) => void | Promise<void>
): vscode.WebviewPanel {
  currentMessageHandler = messageHandler;
  if (!panel) {
    panel = vscode.window.createWebviewPanel(
      "codestation.workspace",
      title,
      vscode.ViewColumn.Active,
      { enableScripts: true, retainContextWhenHidden: true }
    );
    context.subscriptions.push(panel);
    panel.onDidDispose(() => {
      panel = undefined;
      currentMessageHandler = undefined;
    });
    panel.webview.onDidReceiveMessage((message) => currentMessageHandler?.(message));
  } else {
    panel.title = title;
    panel.reveal(vscode.ViewColumn.Active, true);
  }
  panel.webview.html = html;
  return panel;
}

/** Builds the shared CSP-protected HTML document used by editor panels. */
export function documentHtml(title: string, body: string, nonce: string, script = "", extraStyle = ""): string {
  return `<!doctype html><html lang="fr"><head>
  <meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: https:; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
  <title>${title}</title><style nonce="${nonce}">
    body{padding:28px;color:var(--vscode-foreground);font-family:var(--vscode-font-family);line-height:1.55}main{max-width:760px;margin:auto}
    h1{font-size:24px;margin:0 0 16px}h2{font-size:18px;margin-top:28px}a{color:var(--vscode-textLink-foreground)}code{font-family:var(--vscode-editor-font-family);background:var(--vscode-textCodeBlock-background);padding:2px 5px;border-radius:3px}
    pre{padding:14px;overflow:auto;background:var(--vscode-textCodeBlock-background)}pre code{padding:0}.intro,.hint{color:var(--vscode-descriptionForeground)}img{display:block;max-width:100%;height:auto;margin:22px auto;border:1px solid var(--vscode-widget-border);border-radius:6px;background:#fff}
    label{display:block;font-weight:600;margin:16px 0 6px}input{box-sizing:border-box;width:100%;padding:8px 10px;color:var(--vscode-input-foreground);background:var(--vscode-input-background);border:1px solid var(--vscode-input-border,transparent)}
    input:focus{outline:1px solid var(--vscode-focusBorder)}.hint{margin-top:5px;font-size:12px}.actions{display:flex;justify-content:flex-end;margin-top:26px}
    button{border:0;padding:8px 16px;color:var(--vscode-button-foreground);background:var(--vscode-button-background);cursor:pointer}button:hover{background:var(--vscode-button-hoverBackground)}#error{min-height:20px;margin-top:14px;color:var(--vscode-errorForeground)}
    .document-loading{display:flex;min-height:70vh;flex-direction:column;align-items:center;justify-content:center;gap:14px}.document-spinner{width:28px;height:28px;border:3px solid var(--vscode-widget-border);border-top-color:var(--vscode-progressBar-background);border-radius:50%;animation:document-spin .8s linear infinite}@keyframes document-spin{to{transform:rotate(360deg)}}${extraStyle}
  </style></head><body><main>${body}</main>${script ? `<script nonce="${nonce}">${script}</script>` : ""}</body></html>`;
}
