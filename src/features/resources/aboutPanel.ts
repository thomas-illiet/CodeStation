/** Interactive machine inventory rendered in the shared CodeStation panel. */
import * as vscode from "vscode";
import { documentHtml, showCodestationPanel } from "../../core/webviewPanel.js";
import { LocalRegistry } from "../packages/localRegistry.js";
import {
  buildMachineReport,
  collectDevelopmentDependency,
  collectPythonPackages,
  collectSystemPackages,
  failed,
  successful,
  type AboutSnapshot,
  type DevelopmentDependency
} from "./aboutData.js";

type Section = keyof AboutSnapshot;
type AboutMessage = { type?: unknown; section?: unknown; dependency?: unknown };
const MINIMUM_RELOAD_DURATION_MS = 500;

const emptySnapshot = (): AboutSnapshot => ({
  development: { data: [] }, system: { data: [] }, python: { data: [] }, codestation: { data: [] }
});

function nonce(): string { return `${Date.now()}-${Math.random().toString(36).slice(2)}`; }

/** Opens About this Code Station and starts all inventory collectors. */
export function openAboutPanel(context: vscode.ExtensionContext): void {
  const registry = new LocalRegistry();
  const snapshot = emptySnapshot();
  const loading = new Set<string>();
  let started = false;
  let panel!: vscode.WebviewPanel;

  const post = (value: object): void => { void panel.webview.postMessage(value); };
  const setLoading = (key: string, active: boolean): void => {
    if (active) loading.add(key); else loading.delete(key);
    post({ type: "loading", key, active, exportReady: loading.size === 0 });
  };
  const publish = (section: Section): void => post({ type: "inventory", section, value: snapshot[section] });

  const finishLoading = async (key: string, startedAt: number, minimumDurationMs: number): Promise<void> => {
    const remaining = minimumDurationMs - (Date.now() - startedAt);
    if (remaining > 0) await new Promise((resolve) => setTimeout(resolve, remaining));
    setLoading(key, false);
  };
  const loadDependency = async (id: DevelopmentDependency["id"], minimumDurationMs = 0): Promise<void> => {
    const key = `development:${id}`; if (loading.has(key)) return; setLoading(key, true);
    const startedAt = Date.now();
    try {
      const value = await collectDevelopmentDependency(id);
      snapshot.development = successful([...snapshot.development.data.filter((item) => item.id !== id), value].sort((a, b) => a.name.localeCompare(b.name, "en")));
    } catch { snapshot.development = { ...snapshot.development, error: undefined }; }
    publish("development"); await finishLoading(key, startedAt, minimumDurationMs);
  };
  const loadSystem = async (minimumDurationMs = 0): Promise<void> => {
    if (loading.has("system")) return; setLoading("system", true);
    const startedAt = Date.now();
    try { snapshot.system = successful(await collectSystemPackages()); } catch (error) { snapshot.system = failed(snapshot.system, error); }
    publish("system"); await finishLoading("system", startedAt, minimumDurationMs);
  };
  const loadPython = async (minimumDurationMs = 0): Promise<void> => {
    if (loading.has("python")) return; setLoading("python", true);
    const startedAt = Date.now();
    try { snapshot.python = successful(await collectPythonPackages()); } catch (error) { snapshot.python = failed(snapshot.python, error); }
    publish("python"); await finishLoading("python", startedAt, minimumDurationMs);
  };
  const loadCodeStation = async (minimumDurationMs = 0): Promise<void> => {
    if (loading.has("codestation")) return; setLoading("codestation", true);
    const startedAt = Date.now();
    try {
      const records = await registry.list();
      snapshot.codestation = successful(records.sort((a, b) => `${a.namespace}/${a.name}`.localeCompare(`${b.namespace}/${b.name}`, "en")));
    } catch { snapshot.codestation = { ...snapshot.codestation, error: "CodeStation package inventory is unavailable" }; }
    publish("codestation"); await finishLoading("codestation", startedAt, minimumDurationMs);
  };
  const exportReport = async (): Promise<void> => {
    if (loading.size) return;
    const timestamp = new Date().toISOString().replaceAll(":", "-").replace(/\.\d{3}Z$/, "Z");
    const target = await vscode.window.showSaveDialog({
      defaultUri: vscode.Uri.file(`codestation-machine-report-${timestamp}.json`),
      filters: { "JSON report": ["json"] }, saveLabel: "Export report"
    });
    if (!target) return;
    const report = buildMachineReport(snapshot, String(context.extension.packageJSON.version), vscode.version);
    await vscode.workspace.fs.writeFile(target, Buffer.from(`${JSON.stringify(report, null, 2)}\n`, "utf8"));
    void vscode.window.showInformationMessage(`CodeStation machine report exported to ${target.fsPath}`);
  };

  panel = showCodestationPanel(context, "About this Code Station", aboutHtml(nonce()), async (raw) => {
    const message = raw as AboutMessage;
    if (message.type === "ready" && !started) {
      started = true;
      post({ type: "metadata", value: { os: process.platform, architecture: process.arch, codestation: String(context.extension.packageJSON.version), editor: vscode.version } });
      void Promise.all([loadDependency("python"), loadDependency("node"), loadDependency("go"), loadSystem(), loadPython(), loadCodeStation()]);
      return;
    }
    if (message.type === "export") { await exportReport(); return; }
    if (message.type !== "reload") return;
    if (message.section === "development" && ["python", "node", "go"].includes(String(message.dependency))) await loadDependency(message.dependency as DevelopmentDependency["id"], MINIMUM_RELOAD_DURATION_MS);
    if (message.section === "system") await loadSystem(MINIMUM_RELOAD_DURATION_MS);
    if (message.section === "python") await loadPython(MINIMUM_RELOAD_DURATION_MS);
    if (message.section === "codestation") await loadCodeStation(MINIMUM_RELOAD_DURATION_MS);
  });

}

function aboutHtml(valueNonce: string): string {
  const body = `<style nonce="${valueNonce}">${aboutStyles}.center{text-align:center}</style><header class="hero"><div><p class="eyebrow">SYSTEM INFORMATION</p><h1>About this Code Station</h1><p class="intro">Runtime and package inventory for this development environment.</p><p id="machine" class="machine"></p></div><button id="export" class="primary" disabled>Export report</button></header>
  <section class="inventory"><div class="section-title"><div><h2>Development dependencies</h2><p>Paths and versions of the primary development runtimes.</p></div></div><div id="development-error" class="error" role="alert"></div><div class="table-wrap"><table><thead><tr><th>Name</th><th>Path</th><th class="center">Version</th><th class="center">Action</th></tr></thead><tbody id="development-body"></tbody></table></div></section>
  ${listSection("system", "System packages", "All packages registered by Debian.", ["Name", "Version", "Architecture"])}
  ${listSection("python", "Python packages", "Packages installed for the detected Python runtime.", ["Name", "Version"])}
  ${listSection("codestation", "CodeStation packages", "Packages installed and managed by the CodeStation extension.", ["Package", "Version", "Status", "Updated"])} `;
  return documentHtml("About this Code Station", body, valueNonce, aboutScript);
}

function listSection(id: string, title: string, description: string, columns: string[]): string {
  return `<section class="inventory"><div class="section-title"><div><h2>${title}</h2><p>${description}</p></div><button class="reload" data-reload="${id}"><span class="spinner"></span><span>Reload</span></button></div><label class="search"><span class="sr-only">Search ${title}</span><input id="${id}-query" type="search" placeholder="Search ${title.toLowerCase()}"></label><div id="${id}-error" class="error" role="alert"></div><div class="table-wrap"><table><thead><tr>${columns.map((column) => `<th class="${["Name", "Package"].includes(column) ? "" : "center"}">${column}</th>`).join("")}</tr></thead><tbody id="${id}-body"></tbody></table></div><footer class="pager"><span id="${id}-count"></span><div><button data-page="${id}:previous">Previous</button><span id="${id}-page"></span><button data-page="${id}:next">Next</button></div></footer></section>`;
}

const aboutScript = String.raw`
const vscode=acquireVsCodeApi(),PAGE_SIZE=15;
const state={development:[],system:[],python:[],codestation:[]},meta={},loading=new Set(['development:python','development:node','development:go','system','python','codestation']),paging={system:{query:'',page:1},python:{query:'',page:1},codestation:{query:'',page:1}};
const esc=value=>String(value??'').replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
function error(id,value){document.getElementById(id+'-error').textContent=value||''}
function renderDevelopment(){const ids=['python','node','go'],labels={python:'Python',node:'Node.js',go:'Go'},rows=new Map(state.development.map(item=>[item.id,item]));document.getElementById('development-body').innerHTML=ids.map(id=>{const item=rows.get(id),busy=loading.has('development:'+id);return '<tr><td>'+labels[id]+'</td><td class="path">'+esc(item?.path||'Not available')+'</td><td class="center">'+esc(item?.version||'—')+'</td><td class="center"><button class="reload row-reload '+(busy?'busy':'')+'" data-dependency="'+id+'" '+(busy?'disabled':'')+'><span class="spinner"></span><span>Reload</span></button></td></tr>'}).join('');document.querySelectorAll('[data-dependency]').forEach(button=>button.onclick=()=>vscode.postMessage({type:'reload',section:'development',dependency:button.dataset.dependency}))}
function values(id,item){if(id==='system')return[item.name,item.version,item.architecture];if(id==='python')return[item.name,item.version];return[item.name,item.version,item.status,new Date(item.updatedAt).toLocaleString()]}
function renderList(id){const pageState=paging[id],all=state[id],term=pageState.query.trim().toLocaleLowerCase(),filtered=term?all.filter(item=>values(id,item).some(value=>String(value).toLocaleLowerCase().includes(term))):all,totalPages=Math.max(1,Math.ceil(filtered.length/PAGE_SIZE));pageState.page=Math.min(pageState.page,totalPages);const start=(pageState.page-1)*PAGE_SIZE,visible=filtered.slice(start,start+PAGE_SIZE),body=document.getElementById(id+'-body'),columnCount=id==='python'?2:id==='system'?3:4,isLoading=loading.has(id);body.innerHTML=visible.length?visible.map(item=>'<tr>'+values(id,item).map((value,index)=>'<td class="'+(index===0?'':'center')+'">'+esc(value)+'</td>').join('')+'</tr>').join(''):isLoading?'<tr><td class="empty loading-cell busy" colspan="'+columnCount+'"><span class="spinner"></span> Loading…</td></tr>':'<tr><td class="empty" colspan="'+columnCount+'">No matching packages.</td></tr>';document.getElementById(id+'-count').textContent=isLoading&&!all.length?'Loading…':visible.length+' shown / '+filtered.length+' filtered / '+all.length+' total';document.getElementById(id+'-page').textContent='Page '+pageState.page+' of '+totalPages;const previous=document.querySelector('[data-page="'+id+':previous"]'),next=document.querySelector('[data-page="'+id+':next"]');previous.disabled=isLoading||pageState.page<=1;next.disabled=isLoading||pageState.page>=totalPages}
document.querySelectorAll('[data-reload]').forEach(button=>button.onclick=()=>vscode.postMessage({type:'reload',section:button.dataset.reload}));
Object.keys(paging).forEach(id=>{document.getElementById(id+'-query').oninput=event=>{paging[id].query=event.target.value;paging[id].page=1;renderList(id)};document.querySelector('[data-page="'+id+':previous"]').onclick=()=>{paging[id].page--;renderList(id)};document.querySelector('[data-page="'+id+':next"]').onclick=()=>{paging[id].page++;renderList(id)};renderList(id)});
document.getElementById('export').onclick=()=>vscode.postMessage({type:'export'});
window.addEventListener('message',({data})=>{if(data.type==='metadata'){Object.assign(meta,data.value);document.getElementById('machine').textContent=meta.os+' · '+meta.architecture+' · CodeStation '+meta.codestation+' · Editor '+meta.editor;return}if(data.type==='loading'){data.active?loading.add(data.key):loading.delete(data.key);document.getElementById('export').disabled=!data.exportReady;const section=data.key.split(':')[0],button=document.querySelector('[data-reload="'+section+'"]');if(button){const busy=loading.has(section);button.disabled=busy;button.classList.toggle('busy',busy)}section==='development'?renderDevelopment():renderList(section);return}if(data.type==='inventory'){state[data.section]=data.value.data;error(data.section,data.value.error);data.section==='development'?renderDevelopment():renderList(data.section)}});
document.querySelectorAll('[data-reload]').forEach(button=>{button.disabled=true;button.classList.add('busy')});renderDevelopment();vscode.postMessage({type:'ready'});`;

const aboutStyles = `body{padding:38px}main{max-width:1100px}.hero{display:flex;align-items:flex-start;justify-content:space-between;gap:24px;margin-bottom:30px}.hero h1{margin:5px 0 7px;font-size:30px}.eyebrow{margin:0;color:var(--vscode-textLink-foreground);font-size:11px;font-weight:700;letter-spacing:.12em}.machine{margin:9px 0 0;color:var(--vscode-descriptionForeground);font-family:var(--vscode-editor-font-family);font-size:12px}.primary{flex:0 0 auto;margin-top:5px}.inventory{margin:18px 0;padding:20px;border:1px solid var(--vscode-widget-border);border-radius:8px;background:var(--vscode-sideBar-background)}.section-title{display:flex;align-items:center;justify-content:space-between;gap:16px}.section-title h2{margin:0;font-size:17px}.section-title p{margin:3px 0 0;color:var(--vscode-descriptionForeground);font-size:12px}.reload{display:inline-flex;align-items:center;gap:7px;padding:6px 11px}.spinner{display:none;width:12px;height:12px;border:2px solid color-mix(in srgb,var(--vscode-button-foreground) 38%,transparent);border-top-color:var(--vscode-button-foreground);border-radius:50%}.busy .spinner{display:inline-block;animation:spin .8s linear infinite}.search{margin:15px 0 10px}.table-wrap{overflow:auto;border:1px solid var(--vscode-widget-border)}table{width:100%;border-collapse:collapse;font-size:12px}th,td{padding:8px 10px;text-align:left;border-bottom:1px solid var(--vscode-widget-border);white-space:nowrap}th{position:sticky;top:0;background:var(--vscode-editor-background);font-weight:600}tbody tr:last-child td{border-bottom:0}.path{max-width:430px;overflow:hidden;text-overflow:ellipsis;font-family:var(--vscode-editor-font-family)}.row-reload{padding:4px 9px}.error{min-height:0;margin:8px 0;color:var(--vscode-errorForeground);font-size:12px}.error:empty{display:none}.pager{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:11px;color:var(--vscode-descriptionForeground);font-size:11px}.pager div{display:flex;align-items:center;gap:9px}.pager button{padding:4px 9px}.empty{text-align:center;color:var(--vscode-descriptionForeground)}button:disabled{cursor:default;opacity:.55}.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:700px){body{padding:20px}.hero{display:block}.primary{margin-top:15px}.inventory{padding:14px}.section-title{align-items:flex-end}.pager{align-items:flex-start;flex-direction:column}}@media(prefers-reduced-motion:reduce){.busy .spinner{animation:none}}`;
