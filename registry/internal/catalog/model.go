package catalog

import "time"

type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type SizedFileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Signature struct {
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type PackageVersion struct {
	SchemaVersion         int            `json:"schemaVersion"`
	Namespace             string         `json:"namespace"`
	Name                  string         `json:"name"`
	Version               string         `json:"version"`
	Summary               string         `json:"summary"`
	Description           string         `json:"description"`
	Publisher             string         `json:"publisher"`
	Categories            []string       `json:"categories"`
	Platforms             []string       `json:"platforms"`
	Architectures         []string       `json:"architectures"`
	Targets               []string       `json:"targets"`
	TofuConstraint        string         `json:"tofuConstraint"`
	Module                FileRef        `json:"module"`
	Readme                FileRef        `json:"readme"`
	Icon                  FileRef        `json:"icon"`
	Archive               SizedFileRef   `json:"archive"`
	Files                 []SizedFileRef `json:"files"`
	Providers             []string       `json:"providers,omitempty"`
	SensitiveCapabilities []string       `json:"sensitiveCapabilities,omitempty"`
	PublishedAt           time.Time      `json:"publishedAt"`
	Signature             Signature      `json:"signature"`
	Retired               bool           `json:"retired,omitempty"`
}

type PackageSummary struct {
	Namespace     string    `json:"namespace"`
	Name          string    `json:"name"`
	Summary       string    `json:"summary"`
	Publisher     string    `json:"publisher"`
	Categories    []string  `json:"categories"`
	LatestVersion string    `json:"latestVersion"`
	IconURL       string    `json:"iconUrl"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Page struct {
	Items      []PackageSummary `json:"items"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	Total      int              `json:"total"`
	TotalPages int              `json:"totalPages"`
}

type Store interface {
	List(query, category, sortBy string, page, pageSize int) (Page, error)
	Get(namespace, name, version string) (PackageVersion, error)
	Versions(namespace, name string) ([]PackageVersion, error)
	ReadObject(path string) ([]byte, string, error)
}

// AdminStore exposes the only catalogue mutations accepted by the registry API.
// Read operations remain anonymous; callers must authenticate at the HTTP layer.
type AdminStore interface {
	Store
	Publish(manifest PackageVersion, archive, readme, icon []byte) error
	Remove(namespace, name, version string) error
}

// MetadataStore persists and queries package manifests independently of object bytes.
type MetadataStore interface {
	List(query, category, sortBy string, page, pageSize int) (Page, error)
	Get(namespace, name, version string) (PackageVersion, error)
	Versions(namespace, name string) ([]PackageVersion, error)
	InsertMetadata(manifest PackageVersion) error
	DeleteMetadata(namespace, name, version string) error
}

// ObjectStore persists immutable package payloads independently of metadata.
type ObjectStore interface {
	ReadObject(path string) ([]byte, string, error)
	PutObjects(manifest PackageVersion, archive, readme, icon []byte) error
	DeleteObjects(manifest PackageVersion) error
}
