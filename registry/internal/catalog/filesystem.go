package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("not found")

type FileStore struct {
	root string
}

// NewFileStore creates a read-only catalog backed by files below root.
func NewFileStore(root string) *FileStore { return &FileStore{root: root} }

// load decodes the complete local catalog document.
func (s *FileStore) load() ([]PackageVersion, error) {
	b, err := os.ReadFile(filepath.Join(s.root, "catalog.json"))
	if err != nil {
		return nil, err
	}
	var entries []PackageVersion
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	return entries, nil
}

// List filters, sorts and paginates packages from the local catalog.
func (s *FileStore) List(query, category, sortBy string, page, pageSize int) (Page, error) {
	entries, err := s.load()
	if err != nil {
		return Page{}, err
	}
	latest := map[string]PackageVersion{}
	for _, entry := range entries {
		if entry.Retired {
			continue
		}
		key := entry.Namespace + "/" + entry.Name
		if current, ok := latest[key]; !ok || compareSemver(entry.Version, current.Version) > 0 {
			latest[key] = entry
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]PackageSummary, 0, len(latest))
	for _, entry := range latest {
		if q != "" && !strings.Contains(strings.ToLower(entry.Name+" "+entry.Summary+" "+entry.Publisher), q) {
			continue
		}
		if category != "" && !containsFold(entry.Categories, category) {
			continue
		}
		items = append(items, PackageSummary{Namespace: entry.Namespace, Name: entry.Name, Summary: entry.Summary, Publisher: entry.Publisher, Categories: entry.Categories, LatestVersion: entry.Version, IconURL: "/v1/packages/" + entry.Namespace + "/" + entry.Name + "/" + entry.Version + "/icon", UpdatedAt: entry.PublishedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if sortBy == "updated" {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].Name < items[j].Name
	})
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	return Page{Items: items[start:end], Page: page, PageSize: pageSize, Total: total, TotalPages: pages}, nil
}

// Get resolves one version, including the special latest version selector.
func (s *FileStore) Get(namespace, name, version string) (PackageVersion, error) {
	entries, err := s.load()
	if err != nil {
		return PackageVersion{}, err
	}
	var found *PackageVersion
	for i := range entries {
		entry := entries[i]
		if entry.Namespace != namespace || entry.Name != name {
			continue
		}
		if version == "latest" || version == "" {
			if entry.Retired {
				continue
			}
			if found == nil || compareSemver(entry.Version, found.Version) > 0 {
				copy := entry
				found = &copy
			}
		} else if entry.Version == version {
			copy := entry
			found = &copy
			break
		}
	}
	if found == nil {
		return PackageVersion{}, ErrNotFound
	}
	return *found, nil
}

// Versions returns all known releases for a package in descending order.
func (s *FileStore) Versions(namespace, name string) ([]PackageVersion, error) {
	entries, err := s.load()
	if err != nil {
		return nil, err
	}
	result := []PackageVersion{}
	for _, e := range entries {
		if e.Namespace == namespace && e.Name == name {
			result = append(result, e)
		}
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	sort.Slice(result, func(i, j int) bool { return compareSemver(result[i].Version, result[j].Version) > 0 })
	return result, nil
}

// ReadObject loads a catalog object while preventing traversal outside root.
func (s *FileStore) ReadObject(path string) ([]byte, string, error) {
	clean := filepath.Clean(path)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, "", ErrNotFound
	}
	objectsRoot, err := filepath.Abs(filepath.Join(s.root, "objects"))
	if err != nil {
		return nil, "", err
	}
	objectPath := filepath.Join(objectsRoot, clean)
	info, err := os.Lstat(objectPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "", ErrNotFound
	}
	if !strings.HasPrefix(objectPath, objectsRoot+string(filepath.Separator)) {
		return nil, "", ErrNotFound
	}
	b, err := os.ReadFile(objectPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	contentType := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".md":
		contentType = "text/markdown; charset=utf-8"
	case ".png":
		contentType = "image/png"
	case ".svg":
		contentType = "image/svg+xml"
	case ".json":
		contentType = "application/json"
	}
	return b, contentType, nil
}

// containsFold reports whether values contains want using Unicode case folding.
func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// compareSemver compares release versions while ordering prereleases correctly.
func compareSemver(a, b string) int {
	var am, an, ap, bm, bn, bp int
	aParts := strings.SplitN(a, "-", 2)
	bParts := strings.SplitN(b, "-", 2)
	fmt.Sscanf(aParts[0], "%d.%d.%d", &am, &an, &ap)
	fmt.Sscanf(bParts[0], "%d.%d.%d", &bm, &bn, &bp)
	if am != bm {
		if am > bm {
			return 1
		}
		return -1
	}
	if an != bn {
		if an > bn {
			return 1
		}
		return -1
	}
	if ap != bp {
		if ap > bp {
			return 1
		}
		return -1
	}
	if len(aParts) == 1 && len(bParts) > 1 {
		return 1
	}
	if len(aParts) > 1 && len(bParts) == 1 {
		return -1
	}
	if len(aParts) == 1 {
		return 0
	}
	return strings.Compare(aParts[1], bParts[1])
}
