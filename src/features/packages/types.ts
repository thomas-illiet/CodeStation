/** Shared contracts for catalog, manifests, installations and operations. */
/** User-requested mutation applied to a package. */
export type OperationKind = 'install' | 'update' | 'uninstall';
/** Observable states emitted by the serialized operation pipeline. */
export type OperationPhase = 'queued' | 'downloading' | 'verifying' | 'extracting' | 'initializing' | 'planning' | 'applying' | 'verifying-result' | 'succeeded' | 'failed';

/** Immutable file reference included in a signed manifest. */
export interface FileRef { path: string; sha256: string; size?: number }
/** Detached Ed25519 signature and trusted-key identifier. */
export interface Signature { keyId: string; algorithm: 'ed25519'; value: string }
/** Complete signed metadata for one installable package version. */
export interface PackageVersion {
  schemaVersion: 1;
  namespace: string;
  name: string;
  version: string;
  summary: string;
  description: string;
  publisher: string;
  categories: string[];
  platforms: string[];
  architectures: string[];
  targets: Array<'linux-amd64' | 'linux-arm64' | 'windows-amd64'>;
  tofuConstraint: string;
  readme: FileRef;
  icon: FileRef;
  archive: FileRef;
  providers?: string[];
  sensitiveCapabilities?: string[];
  publishedAt: string;
  signature: Signature;
  retired?: boolean;
}

/** Compact package representation returned in catalog pages. */
export interface PackageSummary {
  namespace: string;
  name: string;
  summary: string;
  publisher: string;
  categories: string[];
  latestVersion: string;
  iconUrl: string;
  updatedAt: string;
}

/** Paginated package catalog response. */
export interface CatalogPage {
  items: PackageSummary[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

/** Durable local state for a package managed on this workstation. */
export interface InstalledRecord {
  namespace: string;
  name: string;
  version: string;
  status: 'installed' | 'installing' | 'updating' | 'uninstalling' | 'error';
  installedAt: string;
  updatedAt: string;
  lastOperationId: string;
  lastError?: string;
}

/** Redacted progress event retained for UI replay and local audit logs. */
export interface OperationEvent {
  operationId: string;
  phase: OperationPhase;
  message: string;
  timestamp: string;
  raw?: string;
}
