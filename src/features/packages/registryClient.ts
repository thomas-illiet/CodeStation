/** Size-bounded client for the CodeStation package registry API. */
import * as vscode from 'vscode';
import { CatalogPage, PackageVersion } from './types';

/** Calls registry endpoints with URL, status and response-size validation. */
export class RegistryClient {
  private readonly imageCache = new Map<string, Promise<string>>();
  private base(): URL {
    const configured = vscode.workspace.getConfiguration('codestation').get<string>('registryUrl', '').trim();
    const url = new URL(configured);
    if (!['http:', 'https:'].includes(url.protocol)) throw new Error('CodeStation Packages registry must use HTTP or HTTPS');
    return url;
  }

  async list(query = '', _page = 1, category = '', sort = 'name'): Promise<CatalogPage> {
    const items: CatalogPage['items'] = [];
    let page = 1; let total = 0; let totalPages = 1;
    do {
      const url = new URL('/v1/packages', this.base());
      url.searchParams.set('q', query); url.searchParams.set('page', String(page)); url.searchParams.set('pageSize', '100');
      if (category) url.searchParams.set('category', category);
      url.searchParams.set('sort', sort);
      const result = await this.json<CatalogPage>(url);
      items.push(...result.items); total = result.total; totalPages = result.totalPages; page += 1;
    } while (page <= totalPages);
    return { items, page: 1, pageSize: items.length, total, totalPages: 1 };
  }

  async detail(namespace: string, name: string, version = 'latest'): Promise<PackageVersion> {
    const suffix = version === 'latest' ? '' : `/${encodeURIComponent(version)}`;
    return this.json<PackageVersion>(new URL(`/v1/packages/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}${suffix}`, this.base()));
  }

  async versions(namespace: string, name: string): Promise<PackageVersion[]> {
    const result = await this.json<{ versions: PackageVersion[] }>(new URL(`/v1/packages/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/versions`, this.base()));
    return result.versions;
  }

  url(pathname: string): URL { return new URL(pathname, this.base()); }

  displayName(): string { return this.base().host; }

  async dataUrl(pathname: string, maxBytes = 2_000_000): Promise<string> {
    const cacheKey = `${pathname}:${maxBytes}`;
    const cached = this.imageCache.get(cacheKey);
    if (cached) return cached;
    const pending = this.fetchDataUrl(pathname, maxBytes).catch((error) => {
      this.imageCache.delete(cacheKey);
      throw error;
    });
    if (this.imageCache.size >= 256) this.imageCache.delete(this.imageCache.keys().next().value!);
    this.imageCache.set(cacheKey, pending);
    return pending;
  }

  private async fetchDataUrl(pathname: string, maxBytes: number): Promise<string> {
    const response = await fetch(this.url(pathname), { headers: { Accept: 'image/*' }, redirect: 'error' });
    if (!response.ok) throw new Error(`Registry returned HTTP ${response.status}`);
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > maxBytes) throw new Error('Registry image exceeds maximum allowed size');
    const type = response.headers.get('content-type')?.split(';')[0] || sniffImageType(bytes);
    if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml'].includes(type)) throw new Error(`Unsupported registry image type: ${type}`);
    return `data:${type};base64,${Buffer.from(bytes).toString('base64')}`;
  }

  async download(url: URL, maxBytes: number): Promise<Uint8Array> {
    const response = await fetch(url, { headers: { Accept: 'application/octet-stream' }, redirect: 'error' });
    if (!response.ok) throw new Error(`Registry returned HTTP ${response.status}`);
    const declared = Number(response.headers.get('content-length') || 0);
    if (declared > maxBytes) throw new Error('Package exceeds maximum allowed size');
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > maxBytes) throw new Error('Package exceeds maximum allowed size');
    return bytes;
  }

  async readText(pathname: string, maxBytes = 1_000_000): Promise<string> {
    const data = await this.download(this.url(pathname), maxBytes);
    return new TextDecoder().decode(data);
  }

  private async json<T>(url: URL): Promise<T> {
    const response = await fetch(url, { headers: { Accept: 'application/json' }, redirect: 'error' });
    if (!response.ok) throw new Error(`Registry returned HTTP ${response.status}`);
    return response.json() as Promise<T>;
  }
}

function sniffImageType(bytes: Uint8Array): string {
  const start = new TextDecoder().decode(bytes.slice(0, 256)).trimStart();
  if (start.startsWith('<svg') || start.startsWith('<?xml')) return 'image/svg+xml';
  if (bytes[0] === 0x89 && bytes[1] === 0x50) return 'image/png';
  if (bytes[0] === 0xff && bytes[1] === 0xd8) return 'image/jpeg';
  if (start.startsWith('GIF8')) return 'image/gif';
  if (start.startsWith('RIFF') && start.includes('WEBP')) return 'image/webp';
  return 'application/octet-stream';
}
