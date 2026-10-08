/** Documentation discovery, Markdown rendering and sidebar integration. */
import path from "node:path";
import MarkdownIt from "markdown-it";
import * as vscode from "vscode";
import { documentHtml, showCodestationPanel } from "../../core/webviewPanel.js";

const markdown = new MarkdownIt({ html: false, linkify: true, typographer: true });
/** Searchable metadata for one bundled Markdown document. */
export type DocumentEntry = { title: string; relativePath: string; folder: string };

/** Renders one trusted bundled Markdown document in the shared panel. */
export async function openDocumentation(context: vscode.ExtensionContext, uri: vscode.Uri): Promise<void> {
  const loadingNonce = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const panel = showCodestationPanel(context, "Documentation", documentHtml("Documentation", loadingBody("Loading documentation…"), loadingNonce));
  const source = new TextDecoder().decode(await vscode.workspace.fs.readFile(uri));
  const title = source.match(/^#\s+(.+)$/m)?.[1]?.trim() ?? path.basename(uri.path, ".md");
  const nonce = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  panel.title = title;
  panel.webview.html = documentHtml(title, markdown.render(await inlineLocalImages(source, uri)), nonce);
}

function loadingBody(label: string): string {
  return `<section class="document-loading" role="status"><span class="document-spinner" aria-hidden="true"></span><h1>${label}</h1></section>`;
}

async function inlineLocalImages(source: string, documentUri: vscode.Uri): Promise<string> {
  const matches = [...source.matchAll(/!\[([^\]]*)\]\(([^)\s]+)\)/g)];
  let rendered = source;
  for (const match of matches.reverse()) {
    const target = match[2];
    if (!target || /^(?:data:|https?:)/i.test(target)) continue;
    const segments = decodeURIComponent(target).split("/").filter(Boolean);
    if (!segments.length || segments.some((segment) => segment === "." || segment === "..")) continue;
    const extension = path.extname(target).toLowerCase();
    const mime = extension === ".svg" ? "image/svg+xml" : extension === ".png" ? "image/png" : extension === ".jpg" || extension === ".jpeg" ? "image/jpeg" : undefined;
    if (!mime) continue;
    try {
      const bytes = await vscode.workspace.fs.readFile(vscode.Uri.joinPath(documentUri, "..", ...segments));
      const replacement = `![${match[1]}](data:${mime};base64,${Buffer.from(bytes).toString("base64")})`;
      rendered = rendered.slice(0, match.index) + replacement + rendered.slice((match.index ?? 0) + match[0].length);
    } catch { /* Keep the original Markdown when an optional image is unavailable. */ }
  }
  return rendered;
}

function titleCase(value: string): string {
  return value.replace(/[-_]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

/** Recursively indexes Markdown files below the documentation root. */
export async function collectDocuments(root: vscode.Uri, directory = root, prefix = ""): Promise<DocumentEntry[]> {
  const entries = await vscode.workspace.fs.readDirectory(directory);
  const documents: DocumentEntry[] = [];
  for (const [name, type] of entries.sort(([left], [right]) => left.localeCompare(right, "en"))) {
    const uri = vscode.Uri.joinPath(directory, name);
    const relativePath = prefix ? `${prefix}/${name}` : name;
    if (type === vscode.FileType.Directory) documents.push(...await collectDocuments(root, uri, relativePath));
    if (type !== vscode.FileType.File || !name.toLowerCase().endsWith(".md")) continue;
    const content = new TextDecoder().decode(await vscode.workspace.fs.readFile(uri));
    const title = content.match(/^#\s+(.+)$/m)?.[1]?.trim() ?? titleCase(path.basename(name, path.extname(name)));
    const parent = path.posix.dirname(relativePath);
    documents.push({ title, relativePath, folder: parent === "." ? "General" : parent.split("/").map(titleCase).join(" / ") });
  }
  return documents;
}
