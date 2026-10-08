package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"codestation/registry/internal/catalog"
)

func TestConfigureStoreSelectsSQLite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODESTATION_DATABASE_PROVIDER", "sqlite")
	t.Setenv("CODESTATION_SQLITE_PATH", filepath.Join(root, "metadata", "registry.db"))
	t.Setenv("CODESTATION_OBJECTS_PATH", filepath.Join(root, "packages"))
	store, closeStore, err := configureStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*catalog.CompositeStore); !ok {
		t.Fatalf("store type: %T", store)
	}
	closeStore()
}

func TestConfigureStoreRejectsUnknownProvider(t *testing.T) {
	t.Setenv("CODESTATION_METADATA_PROVIDER", "unknown")
	if _, _, err := configureStore(t.TempDir()); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestConfigureStoreRejectsUnknownObjectProvider(t *testing.T) {
	t.Setenv("CODESTATION_METADATA_PROVIDER", "sqlite")
	t.Setenv("CODESTATION_OBJECT_STORAGE_PROVIDER", "unknown")
	if _, _, err := configureStore(t.TempDir()); err == nil {
		t.Fatal("expected unsupported object provider error")
	}
}

func TestConfigureStoreDecouplesSQLiteAndLocalObjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODESTATION_METADATA_PROVIDER", "sqlite")
	t.Setenv("CODESTATION_OBJECT_STORAGE_PROVIDER", "local")
	t.Setenv("CODESTATION_SQLITE_PATH", filepath.Join(root, "db", "registry.db"))
	t.Setenv("CODESTATION_LOCAL_OBJECTS_PATH", filepath.Join(root, "payloads"))
	store, closeStore, err := configureStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	manifest := catalog.PackageVersion{Namespace: "core", Name: "tool", Version: "1.0.0", PublishedAt: time.Now(), Archive: catalog.SizedFileRef{Path: "core/tool/1.0.0/package.tar.gz"}, Readme: catalog.FileRef{Path: "core/tool/1.0.0/README.md"}, Icon: catalog.FileRef{Path: "core/tool/1.0.0/icon.svg"}}
	admin := store.(catalog.AdminStore)
	if err := admin.Publish(manifest, []byte("archive"), []byte("readme"), []byte("icon")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "payloads", manifest.Archive.Path)); err != nil {
		t.Fatal(err)
	}
}
