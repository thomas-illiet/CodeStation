package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codestation/devkit/internal/packagefile"
)

func TestInitRefusesNonEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	err := initPackage([]string{root, "--namespace", "core", "--name", "demo", "--summary", "Demo", "--publisher", "CodeStation"})
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("error: %v", err)
	}
}
func TestTokenFlagPrecedence(t *testing.T) {
	t.Setenv("CODESTATION_ADMIN_TOKEN", "environment")
	if got := chooseToken("flag"); got != "flag" {
		t.Fatalf("got %q", got)
	}
	if got := chooseToken(""); got != "environment" {
		t.Fatalf("got %q", got)
	}
}
func TestBuildIsDeterministic(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"package.yaml": "schemaVersion: 1\nnamespace: core\nname: demo\nversion: 1.0.0\nsummary: Demo\ndescription: Demo package\npublisher: CodeStation\ncategories: [CLI]\ntargets: [linux-amd64]\ntofuConstraint: '>= 1.8.0'\nmodule: main.tf\nreadme: README.md\nicon: icon.svg\n", "main.tf": "terraform {}\n", "README.md": "# Demo\n", "icon.svg": "<svg/>\n", ".terraform.lock.hcl": "lock\n"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, _, err := packagefile.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := packagefile.Build(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("bundle is not deterministic")
	}
}
