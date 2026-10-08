package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalObjectStore persists immutable package objects below one directory.
type LocalObjectStore struct{ root string }

// NewLocalObjectStore creates the local object directory.
func NewLocalObjectStore(root string) (*LocalObjectStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local object path is required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create object directory: %w", err)
	}
	return &LocalObjectStore{root: root}, nil
}

func (s *LocalObjectStore) ReadObject(path string) ([]byte, string, error) {
	objectPath, err := s.path(path)
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
	return data, objectContentType(objectPath), nil
}

func (s *LocalObjectStore) PutObjects(manifest PackageVersion, archive, readme, icon []byte) error {
	objects := []struct {
		path string
		data []byte
	}{{manifest.Archive.Path, archive}, {manifest.Readme.Path, readme}, {manifest.Icon.Path, icon}}
	written := make([]string, 0, len(objects))
	for _, object := range objects {
		path, err := s.path(object.path)
		if err != nil {
			removeLocalObjects(written)
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			removeLocalObjects(written)
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			removeLocalObjects(written)
			return ErrConflict
		}
		if err != nil {
			removeLocalObjects(written)
			return err
		}
		_, writeErr := file.Write(object.data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			removeLocalObjects(written)
			return errors.Join(writeErr, closeErr)
		}
		written = append(written, path)
	}
	return nil
}

func (s *LocalObjectStore) DeleteObjects(manifest PackageVersion) error {
	for _, object := range []string{manifest.Archive.Path, manifest.Readme.Path, manifest.Icon.Path} {
		path, err := s.path(object)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove object %s: %w", object, err)
		}
	}
	return nil
}

func (s *LocalObjectStore) path(path string) (string, error) {
	clean := filepath.Clean(path)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrNotFound
	}
	return filepath.Join(s.root, clean), nil
}

func removeLocalObjects(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func objectContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".gz":
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}
