package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TestProviderExposesOnlyTypedResources guards the provider's public resource surface.
func TestProviderExposesOnlyTypedResources(t *testing.T) {
	resources := provider().ResourcesMap
	for _, name := range []string{"codestation_binary", "codestation_archive", "codestation_file", "codestation_npm_package"} {
		if resources[name] == nil {
			t.Fatalf("missing resource %s", name)
		}
	}
	if len(resources) != 4 {
		t.Fatalf("unexpected resources: %#v", resources)
	}
}

// TestSafeDestinationRejectsOutsideHome verifies the home-directory boundary.
func TestSafeDestinationRejectsOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := safeDestination(filepath.Join(home, "bin", "tool")); err != nil {
		t.Fatal(err)
	}
	if _, err := safeDestination(filepath.Join(home, "..", "tool")); err == nil {
		t.Fatal("expected destination outside home to be rejected")
	}
}

// TestExtractTarMember verifies deterministic extraction of one archive member.
func TestExtractTarMember(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	body := []byte("managed binary")
	if err := tw.WriteHeader(&tar.Header{Name: "bin/tool", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := extractMember(archive.Bytes(), "https://example.test/tool.tar.gz", "bin/tool")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
}

// TestDeleteRefusesModifiedFile ensures uninstall never deletes user-modified content.
func TestDeleteRefusesModifiedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := schema.TestResourceDataRaw(t, resourceFile().Schema, map[string]interface{}{"version": "1", "destination": path, "content": "original", "installed_sha256": digest([]byte("original"))})
	data.SetId(path)
	if diagnostics := deleteManagedFile(context.Background(), data, nil); !diagnostics.HasError() {
		t.Fatal("expected modified file deletion to fail")
	}
}
