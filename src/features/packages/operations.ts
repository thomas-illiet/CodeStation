/** Serialized, observable package installation lifecycle. */
import { promises as fs } from 'node:fs';
import * as path from 'node:path';
import { randomUUID } from 'node:crypto';
import * as vscode from 'vscode';
import { extractTarGz } from './archive';
import { LocalRegistry } from './localRegistry';
import { appendOperationLog, backupState, ensureOperationDirectories, protectWorkspace, removeStaleLock } from './operationStorage.js';
import { paths } from './paths';
import { processStartId } from './processIdentity.js';
import { RegistryClient } from './registryClient';
import { redact, sha256, verifyManifest } from './security';
import { TofuRunner } from './tofu';
import { InstalledRecord, OperationEvent, OperationKind, PackageVersion } from './types';

type Listener = (event: OperationEvent) => void;
interface Request { kind: OperationKind; manifest: PackageVersion; operationId: string }

/** Serializes package mutations and publishes their complete event history. */
export class OperationManager implements vscode.Disposable {
  private readonly listeners = new Set<Listener>();
  private readonly history = new Map<string, OperationEvent[]>();
  private readonly requests = new Map<string, Request>();
  private queue: Request[] = [];
  private running = false;

  constructor(private readonly extensionRoot: string, private readonly client: RegistryClient, private readonly local: LocalRegistry) {}
  dispose(): void { this.listeners.clear(); }
  subscribe(listener: Listener): vscode.Disposable { this.listeners.add(listener); return { dispose: () => this.listeners.delete(listener) }; }
  events(operationId: string): OperationEvent[] { return this.history.get(operationId) || []; }
  operation(operationId: string): Request | undefined { return this.requests.get(operationId); }
  operations(): Request[] { return [...this.requests.values()].reverse(); }

  enqueue(kind: OperationKind, manifest: PackageVersion): string {
    const operationId = randomUUID(); this.queue.push({ kind, manifest, operationId });
    this.requests.set(operationId, { kind, manifest, operationId });
    this.emit({ operationId, phase: 'queued', message: `${kind} queued`, timestamp: new Date().toISOString() });
    void this.drain(); return operationId;
  }

  private async drain(): Promise<void> {
    if (this.running) return; this.running = true;
    try { while (this.queue.length) { const request=this.queue.shift()!; try { await this.execute(request); } catch (error) { await this.fail(request,error); } } }
    finally { this.running=false; }
  }

  private async execute(request: Request): Promise<void> {
    await ensureOperationDirectories(); await this.acquireLock(request.operationId); const workDir=paths.package(request.manifest.namespace,request.manifest.name);
    try {
      await fs.mkdir(workDir,{recursive:true,mode:0o700});
      const now=new Date().toISOString(); const existing=await this.local.get(request.manifest.namespace,request.manifest.name);
      await this.local.save({ namespace:request.manifest.namespace,name:request.manifest.name,version:existing?.version||request.manifest.version,status:request.kind==='uninstall'?'uninstalling':request.kind==='update'?'updating':'installing',installedAt:existing?.installedAt||now,updatedAt:now,lastOperationId:request.operationId });
      if (request.kind!=='uninstall') await this.stage(request,workDir);
      await backupState(workDir);
      const registryUrl=vscode.workspace.getConfiguration('codestation').get<string>('registryUrl')!; const tofu=new TofuRunner(this.extensionRoot,registryUrl); const configDir=path.join(workDir,'config');
      this.phase(request,'initializing','Initializing the offline OpenTofu workspace'); await tofu.run(['init','-input=false','-no-color','-json'],configDir,request.operationId,'initializing',e=>this.emit(e));
      this.phase(request,'planning','Calculating local changes'); const plan=path.join(workDir,'codestation.tfplan'); const state=path.join(configDir,'terraform.tfstate'); const destroy=request.kind==='uninstall'?['-destroy']:[];
      await tofu.run(['plan','-input=false','-no-color','-json',`-state=${state}`,`-out=${plan}`,...destroy],configDir,request.operationId,'planning',e=>this.emit(e));
      this.phase(request,'applying',request.kind==='uninstall'?'Removing package resources':'Applying package changes');
      await tofu.run(['apply','-input=false','-auto-approve','-no-color','-json',plan],configDir,request.operationId,'applying',e=>this.emit(e));
      this.phase(request,'verifying-result','Verifying final state');
      if(request.kind==='uninstall'){await this.local.remove(request.manifest.namespace,request.manifest.name)}else{await this.local.save({namespace:request.manifest.namespace,name:request.manifest.name,version:request.manifest.version,status:'installed',installedAt:existing?.installedAt||now,updatedAt:new Date().toISOString(),lastOperationId:request.operationId})}
      this.phase(request,'succeeded',request.kind==='uninstall'?'Package removed successfully':`Package ${request.manifest.version} installed successfully`);
    } finally { try { await protectWorkspace(workDir); } finally { await this.releaseLock(request.operationId); } }
  }

  private async stage(request: Request, workDir: string): Promise<void> {
    this.phase(request,'verifying','Verifying signed package metadata'); await verifyManifest(request.manifest,this.extensionRoot);
    const disk=await fs.statfs(paths.cache);const required=(request.manifest.archive.size||0)+50*1024*1024;if(disk.bavail*disk.bsize<required)throw new Error('Insufficient disk space for package staging and state backup');
    this.phase(request,'downloading','Downloading immutable package archive');
    const url=this.client.url(`/v1/packages/${request.manifest.namespace}/${request.manifest.name}/${request.manifest.version}/download`); const bytes=await this.client.download(url,request.manifest.archive.size||536_870_912);
    if(bytes.byteLength!==request.manifest.archive.size)throw new Error('Downloaded package size does not match the manifest');if(sha256(bytes)!==request.manifest.archive.sha256)throw new Error('Downloaded package digest does not match the manifest');
    const archive=path.join(paths.cache,`${request.manifest.namespace}-${request.manifest.name}-${request.manifest.version}.tar.gz`);await fs.mkdir(paths.cache,{recursive:true,mode:0o700});await fs.writeFile(archive,bytes,{mode:0o600});
    this.phase(request,'extracting','Extracting package into its protected workspace');
    const configDir=path.join(workDir,'config');const staged=path.join(workDir,`.staged-${request.operationId}`);await fs.rm(staged,{recursive:true,force:true});await extractTarGz(archive,staged);await fs.rm(configDir,{recursive:true,force:true});await fs.rename(staged,configDir);
  }

  private async fail(request: Request, error: unknown): Promise<void> { const message=redact(error instanceof Error?error.message:String(error));const existing=await this.local.get(request.manifest.namespace,request.manifest.name);if(existing)await this.local.save({...existing,status:'error',updatedAt:new Date().toISOString(),lastOperationId:request.operationId,lastError:message});this.phase(request,'failed',message) }
  private phase(request: Request,phase:OperationEvent['phase'],message:string):void{this.emit({operationId:request.operationId,phase,message,timestamp:new Date().toISOString()})}
  private emit(event:OperationEvent):void{const events=this.history.get(event.operationId)||[];events.push(event);this.history.set(event.operationId,events);for(const listener of this.listeners)listener(event);void appendOperationLog(event)}
  private async acquireLock(operationId:string):Promise<void>{try{const handle=await fs.open(paths.lock(),'wx',0o600);await handle.writeFile(JSON.stringify({operationId,pid:process.pid,processStartId:await processStartId(process.pid),startedAt:new Date().toISOString()}));await handle.close()}catch(error){if((error as NodeJS.ErrnoException).code==='EEXIST'){if(await removeStaleLock()){return this.acquireLock(operationId)}throw new Error('Another CodeStation Packages window is already running an operation')}throw error}}
  private async releaseLock(operationId:string):Promise<void>{try{const lock=JSON.parse(await fs.readFile(paths.lock(),'utf8')) as {operationId:string};if(lock.operationId===operationId)await fs.unlink(paths.lock())}catch(error){if((error as NodeJS.ErrnoException).code!=='ENOENT')throw error}}
}
