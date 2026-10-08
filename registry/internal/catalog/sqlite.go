package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS package_versions (
    namespace TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    manifest TEXT NOT NULL,
    retired INTEGER NOT NULL DEFAULT 0,
    published_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (namespace, name, version)
);
CREATE INDEX IF NOT EXISTS package_versions_latest_idx
    ON package_versions (namespace, name, published_at DESC);
`

// SQLiteConfig configures the self-contained SQLite and filesystem backend.
type SQLiteConfig struct {
	DatabasePath string
	ObjectsPath  string
}

// SQLiteStore stores searchable metadata in SQLite and immutable objects on disk.
type SQLiteStore struct {
	db          *sql.DB
	objectsPath string
}

// NewSQLiteStore opens the database and creates its schema and object directory.
func NewSQLiteStore(config SQLiteConfig) (*SQLiteStore, error) {
	if strings.TrimSpace(config.ObjectsPath) == "" {
		return nil, errors.New("sqlite object path is required")
	}
	return newSQLiteStore(config.DatabasePath, config.ObjectsPath)
}

// NewSQLiteMetadataStore opens an independent SQLite metadata provider.
func NewSQLiteMetadataStore(databasePath string) (*SQLiteStore, error) {
	return newSQLiteStore(databasePath, "")
}

func newSQLiteStore(databasePath, objectsPath string) (*SQLiteStore, error) {
	if strings.TrimSpace(databasePath) == "" {
		return nil, errors.New("sqlite database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0700); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	if objectsPath != "" {
		if err := os.MkdirAll(objectsPath, 0700); err != nil {
			return nil, fmt.Errorf("create object directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;` + sqliteSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite: %w", err)
	}
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			db.Close()
			return nil, fmt.Errorf("secure sqlite file: %w", err)
		}
	}
	return &SQLiteStore{db: db, objectsPath: objectsPath}, nil
}

// Close releases the SQLite connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// List filters, sorts, and paginates the latest non-retired releases.
func (s *SQLiteStore) List(query, category, sortBy string, page, pageSize int) (Page, error) {
	rows, err := s.db.Query(`SELECT manifest FROM package_versions WHERE retired = 0 ORDER BY published_at DESC`)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	latest := map[string]PackageVersion{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return Page{}, err
		}
		var entry PackageVersion
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return Page{}, err
		}
		key := entry.Namespace + "/" + entry.Name
		if _, exists := latest[key]; !exists {
			latest[key] = entry
		}
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]PackageSummary, 0, len(latest))
	for _, entry := range latest {
		if q != "" && !strings.Contains(strings.ToLower(entry.Namespace+" "+entry.Name+" "+entry.Summary+" "+entry.Publisher), q) {
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
		if items[i].Name == items[j].Name {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := min(start+pageSize, total)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return Page{Items: items[start:end], Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

// Get returns one release or the latest non-retired release.
func (s *SQLiteStore) Get(namespace, name, version string) (PackageVersion, error) {
	query := `SELECT manifest FROM package_versions WHERE namespace = ? AND name = ? AND version = ?`
	args := []any{namespace, name, version}
	if version == "" || version == "latest" {
		query = `SELECT manifest FROM package_versions WHERE namespace = ? AND name = ? AND retired = 0 ORDER BY published_at DESC LIMIT 1`
		args = []any{namespace, name}
	}
	var raw string
	if err := s.db.QueryRow(query, args...).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return PackageVersion{}, ErrNotFound
	} else if err != nil {
		return PackageVersion{}, err
	}
	var result PackageVersion
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return PackageVersion{}, err
	}
	return result, nil
}

// Versions returns all releases in descending publication order.
func (s *SQLiteStore) Versions(namespace, name string) ([]PackageVersion, error) {
	rows, err := s.db.Query(`SELECT manifest FROM package_versions WHERE namespace = ? AND name = ? ORDER BY published_at DESC`, namespace, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PackageVersion{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entry PackageVersion
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, ErrNotFound
	}
	return result, nil
}

// ReadObject reads an immutable object while preventing path traversal.
func (s *SQLiteStore) ReadObject(path string) ([]byte, string, error) {
	objectPath, err := s.objectPath(path)
	if err != nil {
		return nil, "", err
	}
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
	data, err := os.ReadFile(objectPath)
	if err != nil {
		return nil, "", err
	}
	contentType := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(objectPath)) {
	case ".md":
		contentType = "text/markdown; charset=utf-8"
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	case ".json":
		contentType = "application/json"
	case ".gz":
		contentType = "application/gzip"
	}
	return data, contentType, nil
}

// Publish persists immutable objects before atomically inserting their metadata.
func (s *SQLiteStore) Publish(manifest PackageVersion, archive, readme, icon []byte) error {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM package_versions WHERE namespace = ? AND name = ? AND version = ?`, manifest.Namespace, manifest.Name, manifest.Version).Scan(&exists)
	if err == nil {
		return ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	objects := []struct {
		path string
		data []byte
	}{{manifest.Archive.Path, archive}, {manifest.Readme.Path, readme}, {manifest.Icon.Path, icon}}
	written := make([]string, 0, len(objects))
	for _, object := range objects {
		path, err := s.objectPath(object.path)
		if err != nil {
			s.removeWritten(written)
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			s.removeWritten(written)
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			s.removeWritten(written)
			return ErrConflict
		}
		if err != nil {
			s.removeWritten(written)
			return err
		}
		_, writeErr := file.Write(object.data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			s.removeWritten(written)
			return errors.Join(writeErr, closeErr)
		}
		written = append(written, path)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		s.removeWritten(written)
		return err
	}
	_, err = s.db.Exec(`INSERT INTO package_versions(namespace, name, version, manifest, retired, published_at) VALUES(?, ?, ?, ?, ?, ?)`, manifest.Namespace, manifest.Name, manifest.Version, string(raw), manifest.Retired, manifest.PublishedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		s.removeWritten(written)
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return ErrConflict
		}
	}
	return err
}

// InsertMetadata inserts one immutable package manifest.
func (s *SQLiteStore) InsertMetadata(manifest PackageVersion) error {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO package_versions(namespace, name, version, manifest, retired, published_at) VALUES(?, ?, ?, ?, ?, ?)`, manifest.Namespace, manifest.Name, manifest.Version, string(raw), manifest.Retired, manifest.PublishedAt.UTC().Format(time.RFC3339Nano))
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return ErrConflict
	}
	return err
}

// DeleteMetadata deletes one package manifest.
func (s *SQLiteStore) DeleteMetadata(namespace, name, version string) error {
	result, err := s.db.Exec(`DELETE FROM package_versions WHERE namespace = ? AND name = ? AND version = ?`, namespace, name, version)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Remove deletes a release and its objects.
func (s *SQLiteStore) Remove(namespace, name, version string) error {
	manifest, err := s.Get(namespace, name, version)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM package_versions WHERE namespace = ? AND name = ? AND version = ?`, namespace, name, version)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	for _, object := range []string{manifest.Archive.Path, manifest.Readme.Path, manifest.Icon.Path} {
		path, pathErr := s.objectPath(object)
		if pathErr != nil {
			return pathErr
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove object %s: %w", object, err)
		}
	}
	return nil
}

func (s *SQLiteStore) objectPath(path string) (string, error) {
	clean := filepath.Clean(path)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrNotFound
	}
	return filepath.Join(s.objectsPath, clean), nil
}

func (s *SQLiteStore) removeWritten(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
