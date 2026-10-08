/** TCP port connectivity test backed by netcat. */
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import * as vscode from "vscode";
import { documentHtml, showCodestationPanel } from "../../core/webviewPanel.js";
import { validPortTarget } from "./portTestValidation.js";

const execFileAsync = promisify(execFile);
interface PortTestMessage { type?: unknown; host?: unknown; port?: unknown }

async function runPortTest(host: string, port: number): Promise<string> {
  try {
    const result = await execFileAsync("nc", ["-vz", host, String(port)], { timeout: 10_000, maxBuffer: 64 * 1024, encoding: "utf8" });
    return (result.stderr || result.stdout || `Connection to ${host}:${port} succeeded.`).trim();
  } catch (error) {
    const failure = error as Error & { stdout?: string; stderr?: string; code?: string | number };
    if (failure.code === "ENOENT") throw new Error("netcat (nc) is not installed in this environment.");
    const detail = (failure.stderr || failure.stdout || failure.message).trim();
    throw new Error(detail || `Connection to ${host}:${port} failed.`);
  }
}

/** Opens a validated host/port form and runs nc -vz without a shell. */
export function openPortTest(context: vscode.ExtensionContext): void {
  const nonce = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const body = `<h1>Test TCP port</h1><p class="intro">Check whether a TCP service is reachable with <code>nc -vz</code>.</p>
  <form id="port-form"><label for="host">Host</label><input id="host" type="text" required placeholder="registry.internal.example" autocomplete="off">
  <label for="port">Port</label><input id="port" type="number" required min="1" max="65535" placeholder="443" inputmode="numeric">
  <div id="error" role="alert"></div><div class="actions"><button id="submit" type="submit">Test port</button></div></form>`;
  const script = `const vscode=acquireVsCodeApi(),form=document.getElementById('port-form'),error=document.getElementById('error'),button=document.getElementById('submit');form.addEventListener('submit',event=>{event.preventDefault();const host=document.getElementById('host').value.trim(),port=Number(document.getElementById('port').value);error.textContent='';button.disabled=true;button.textContent='Testing…';vscode.postMessage({type:'test',host,port})});window.addEventListener('message',({data})=>{if(data.type!=='portResult')return;button.disabled=false;button.textContent='Test port';error.style.color=data.ok?'var(--vscode-testing-iconPassed)':'var(--vscode-errorForeground)';error.textContent=data.message});`;

  const panel = showCodestationPanel(context, "Test TCP port", documentHtml("Test TCP port", body, nonce, script), async (raw) => {
    const message = raw as PortTestMessage;
    if (message.type !== "test") return;
    const host = typeof message.host === "string" ? message.host.trim() : "";
    const port = typeof message.port === "number" ? message.port : Number.NaN;
    if (!validPortTarget(host, port)) {
      await panel.webview.postMessage({ type: "portResult", ok: false, message: "Enter a valid host and a port between 1 and 65535." });
      return;
    }
    try {
      const output = await runPortTest(host, port);
      await panel.webview.postMessage({ type: "portResult", ok: true, message: output });
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error);
      await panel.webview.postMessage({ type: "portResult", ok: false, message: detail });
    }
  });
}
