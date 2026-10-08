/** Secure HTTP proxy configuration backed by VS Code secret storage. */
import * as vscode from "vscode";
import * as http from "node:http";
import * as https from "node:https";
import { performance } from "node:perf_hooks";
import { documentHtml, showCodestationPanel } from "../../core/webviewPanel.js";

const SECRET_KEY = "codestation.proxy.password";
const STATE_KEY = "codestation.proxy.configuration";
const PROXY_KEYS = ["HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"] as const;

interface ProxyConfiguration { url: string; username: string; noProxy: string }
interface ProxyFormMessage { type: "save"; url?: unknown; username?: unknown; password?: unknown }

function escapeHtml(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function authenticatedUrl(configuration: ProxyConfiguration, password: string): string {
  const url = new URL(configuration.url);
  if (configuration.username) {
    url.username = configuration.username;
    url.password = password;
  }
  return url.toString();
}

async function removeLegacyTerminalSettings(): Promise<void> {
  const terminal = vscode.workspace.getConfiguration("terminal.integrated");
  const environment = { ...terminal.get<Record<string, string | null>>("env.linux", {}) };
  let changed = false;
  for (const key of PROXY_KEYS) {
    if (key in environment) { delete environment[key]; changed = true; }
  }
  if (changed) await terminal.update("env.linux", environment, vscode.ConfigurationTarget.Global);
}

/** Applies proxy credentials previously saved in VS Code secret storage. */
export async function applyStoredProxy(context: vscode.ExtensionContext): Promise<void> {
  const configuration = context.globalState.get<ProxyConfiguration>(STATE_KEY);
  const password = await context.secrets.get(SECRET_KEY);
  context.environmentVariableCollection.clear();
  if (!configuration) return;
  const proxy = authenticatedUrl(configuration, password ?? "");
  context.environmentVariableCollection.description = "HTTP proxy managed by CodeStation";
  for (const key of ["HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"]) {
    context.environmentVariableCollection.replace(key, proxy);
  }
  context.environmentVariableCollection.replace("NO_PROXY", configuration.noProxy);
  context.environmentVariableCollection.replace("no_proxy", configuration.noProxy);
}

/** Opens the proxy configuration form without exposing stored credentials. */
export function openProxyModal(context: vscode.ExtensionContext): void {
  const current = context.globalState.get<ProxyConfiguration>(STATE_KEY) ?? {
    url: vscode.workspace.getConfiguration("http").get<string>("proxy", ""),
    username: "",
    noProxy: "localhost,127.0.0.1"
  };
  const nonce = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const body = `<h1>Configure HTTP proxy</h1>
  <p class="intro">This configuration will be used by VS Code and new terminals.</p>
  <form id="proxy-form"><label for="url">Proxy URL</label><input id="url" type="url" required placeholder="http://proxy.internal.example:3128" value="${escapeHtml(current.url)}" autocomplete="off">
  <label for="username">Username</label><input id="username" type="text" placeholder="first.last" value="${escapeHtml(current.username)}" autocomplete="username">
  <label for="password">Password</label><input id="password" type="password" placeholder="${current.username ? "Leave blank to keep the current password" : "Proxy password"}" autocomplete="current-password">
  <p class="hint">The password is stored in VS Code secret storage and never appears in settings.</p><div id="error" role="alert"></div><div class="actions"><button type="submit">Save</button></div></form>
  `;
  const script = `const vscode=acquireVsCodeApi(),form=document.getElementById('proxy-form'),error=document.getElementById('error');form.addEventListener('submit',event=>{event.preventDefault();error.textContent='';const url=document.getElementById('url').value.trim(),username=document.getElementById('username').value.trim(),password=document.getElementById('password').value;if(!/^https?:\\/\\//i.test(url)){error.textContent='The URL must start with http:// or https://';return}vscode.postMessage({type:'save',url,username,password})});`;

  showCodestationPanel(context, "Configure HTTP proxy", documentHtml("Configure HTTP proxy", body, nonce, script), async (rawMessage) => {
    const message = rawMessage as ProxyFormMessage;
    if (message.type !== "save") return;
    const url = typeof message.url === "string" ? message.url.trim() : "";
    const username = typeof message.username === "string" ? message.username.trim() : "";
    const password = typeof message.password === "string" ? message.password : "";
    try {
      const parsed = new URL(url);
      if (!(["http:", "https:"] as string[]).includes(parsed.protocol)) throw new Error("protocol");
      parsed.username = ""; parsed.password = "";
      const configuration: ProxyConfiguration = { url: parsed.toString(), username, noProxy: "localhost,127.0.0.1" };
      await removeLegacyTerminalSettings();
      await vscode.workspace.getConfiguration("http").update("proxy", configuration.url, vscode.ConfigurationTarget.Global);
      await context.globalState.update(STATE_KEY, configuration);
      if (password) await context.secrets.store(SECRET_KEY, password);
      if (!username) await context.secrets.delete(SECRET_KEY);
      await applyStoredProxy(context);
      void vscode.window.showInformationMessage("Proxy configured. Open a new terminal to use it.");
    } catch { void vscode.window.showErrorMessage("The proxy URL is invalid."); }
  });
}

/** Removes CodeStation-managed proxy settings and stored credentials. */
export async function clearHttpProxy(context: vscode.ExtensionContext): Promise<void> {
  const choice = await vscode.window.showWarningMessage("Remove the proxy configuration from VS Code and new terminals?", { modal: true }, "Remove");
  if (choice !== "Remove") return;
  await removeLegacyTerminalSettings();
  await Promise.all([
    vscode.workspace.getConfiguration("http").update("proxy", undefined, vscode.ConfigurationTarget.Global),
    context.globalState.update(STATE_KEY, undefined),
    context.secrets.delete(SECRET_KEY)
  ]);
  context.environmentVariableCollection.clear();
  void vscode.window.showInformationMessage("Proxy configuration removed from VS Code and new terminals.");
}

/** Verifies that the configured proxy accepts an authenticated outbound tunnel. */
export async function testHttpProxy(context: vscode.ExtensionContext): Promise<void> {
  const configuration = context.globalState.get<ProxyConfiguration>(STATE_KEY);
  if (!configuration) {
    void vscode.window.showWarningMessage("Configure an HTTP proxy before testing it.");
    return;
  }

  const targetValue = vscode.workspace.getConfiguration("codestation").get<string>("proxyTestUrl", "https://example.com/");
  let proxy: URL; let target: URL;
  try {
    proxy = new URL(configuration.url);
    target = new URL(targetValue);
    if (!["http:", "https:"].includes(target.protocol)) throw new Error("unsupported target protocol");
  } catch {
    void vscode.window.showErrorMessage("The proxy or proxy test URL is invalid.");
    return;
  }

  const password = await context.secrets.get(SECRET_KEY) ?? "";
  const headers: http.OutgoingHttpHeaders = { Host: `${target.hostname}:${target.port || (target.protocol === "https:" ? "443" : "80")}` };
  if (configuration.username) headers["Proxy-Authorization"] = `Basic ${Buffer.from(`${configuration.username}:${password}`).toString("base64")}`;
  const started = performance.now();

  await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: "CodeStation: testing HTTP proxy" }, () => new Promise<void>((resolve) => {
    const requestFn = proxy.protocol === "https:" ? https.request : http.request;
    const request = requestFn({
      protocol: proxy.protocol,
      hostname: proxy.hostname,
      port: proxy.port || (proxy.protocol === "https:" ? 443 : 80),
      method: "CONNECT",
      path: `${target.hostname}:${target.port || (target.protocol === "https:" ? "443" : "80")}`,
      headers,
      timeout: 10_000
    });
    request.once("connect", (response, socket) => {
      socket.destroy();
      const duration = Math.round(performance.now() - started);
      if (response.statusCode === 200) void vscode.window.showInformationMessage(`HTTP proxy test succeeded in ${duration} ms.`);
      else void vscode.window.showErrorMessage(`HTTP proxy test failed with status ${response.statusCode ?? "unknown"}.`);
      resolve();
    });
    request.once("timeout", () => request.destroy(new Error("connection timed out")));
    request.once("error", (error) => { void vscode.window.showErrorMessage(`HTTP proxy test failed: ${error.message}`); resolve(); });
    request.end();
  }));
}
