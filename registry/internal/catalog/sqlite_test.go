package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreLifecycleAndPersistence(t *testing.T) {
	root := t.TempDir()
	config := SQLiteConfig{DatabasePath: filepath.Join(root, "registry.db"), ObjectsPath: filepath.Join(root, "objects")}
	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(config.DatabasePath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("database permissions: info=%v err=%v", info, err)
	}
	manifest := PackageVersion{
		SchemaVersion: 1, Namespace: "core", Name: "hello", Version: "1.2.0",
		Summary: "Hello package", Publisher: "Platform", Categories: []string{"CLI"},
		PublishedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Archive:     SizedFileRef{Path: "core/hello/1.2.0/package.tar.gz"},
		Readme:      FileRef{Path: "core/hello/1.2.0/README.md"},
		Icon:        FileRef{Path: "core/hello/1.2.0/icon.svg"},
	}
	if err := store.Publish(manifest, []byte("archive"), []byte("# Hello"), []byte("<svg/>")); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(manifest, nil, nil, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate publish: %v", err)
	}
	page, err := store.List("hello", "cli", "updated", 1, 20)
	if err != nil || page.Total != 1 || page.Items[0].LatestVersion != "1.2.0" {
		t.Fatalf("list: page=%+v err=%v", page, err)
	}
	data, contentType, err := store.ReadObject(manifest.Readme.Path)
	if err != nil || string(data) != "# Hello" || contentType != "text/markdown; charset=utf-8" {
		t.Fatalf("read object: data=%q type=%q err=%v", data, contentType, err)
	}
	if _, _, err := store.ReadObject("../registry.db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("path traversal: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = NewSQLiteStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get("core", "hello", "latest")
	if err != nil || got.Version != manifest.Version {
		t.Fatalf("persistent get: got=%+v err=%v", got, err)
	}
	if err := store.Remove("core", "hello", "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("core", "hello", "1.2.0"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed metadata: %v", err)
	}
	if _, _, err := store.ReadObject(manifest.Readme.Path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed object: %v", err)
	}
}
