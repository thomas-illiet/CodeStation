package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codestation/registry/internal/catalog"
)

type adminTestStore struct {
	removed   bool
	published catalog.PackageVersion
}

// List implements catalog.Store for authorization tests.
func (s *adminTestStore) List(string, string, string, int, int) (catalog.Page, error) {
	return catalog.Page{}, nil
}

// Get returns no package from the authorization-test store.
func (s *adminTestStore) Get(string, string, string) (catalog.PackageVersion, error) {
	return catalog.PackageVersion{}, catalog.ErrNotFound
}

// Versions returns no releases from the authorization-test store.
func (s *adminTestStore) Versions(string, string) ([]catalog.PackageVersion, error) {
	return nil, catalog.ErrNotFound
}

// ReadObject returns no objects from the authorization-test store.
func (s *adminTestStore) ReadObject(string) ([]byte, string, error) {
	return nil, "", catalog.ErrNotFound
}

// Publish accepts uploads so tests can isolate authorization behavior.
func (s *adminTestStore) Publish(manifest catalog.PackageVersion, _ []byte, _ []byte, _ []byte) error {
	s.published = manifest
	return nil
}

// Remove records that the authenticated mutation reached the store.
func (s *adminTestStore) Remove(string, string, string) error {
	s.removed = true
	return nil
}

// TestCatalogAndModuleProtocol covers catalog reads and the OpenTofu module protocol.
func TestCatalogAndModuleProtocol(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "objects"), 0700); err != nil {
		t.Fatal(err)
	}
	readme := []byte("# Hello")
	sum := "01c8de44e04d2f7a304f50963545a2aff58c33e9c44a1f33fdcb978fb224cb74"
	if err := os.WriteFile(filepath.Join(root, "objects", "README.md"), readme, 0600); err != nil {
		t.Fatal(err)
	}
	catalogJSON := `[{"schemaVersion":1,"namespace":"core","name":"hello","version":"1.2.0","summary":"Hello","description":"Test","publisher":"Platform","categories":["CLI"],"platforms":["linux"],"architectures":["arm64"],"tofuConstraint":">= 1.8","readme":{"path":"README.md","sha256":"` + sum + `"},"icon":{"path":"README.md","sha256":"` + sum + `"},"archive":{"path":"README.md","sha256":"` + sum + `","size":7},"publishedAt":"` + time.Now().UTC().Format(time.RFC3339) + `","signature":{"keyId":"test","algorithm":"ed25519","value":"x"}}]`
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), []byte(catalogJSON), 0600); err != nil {
		t.Fatal(err)
	}
	handler := New(catalog.NewFileStore(root), slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	for _, path := range []string{"/v1/packages?q=hello", "/v1/packages/core/hello", "/v1/packages/core/hello/versions", "/v1/modules/core/hello/codestation/versions"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/modules/core/hello/codestation/1.2.0/download", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatalf("status: %d", response.Code)
	}
	if !strings.Contains(response.Header().Get("X-Terraform-Get"), "download") {
		t.Fatal("missing module download header")
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/packages/core/hello/1.2.0/readme", nil)
	request.Header.Set("Range", "bytes=0-2")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "# H" {
		t.Fatalf("range response: status=%d body=%q", response.Code, response.Body.String())
	}
}

// TestAdminTokenOnlyProtectsMutationRoutes verifies the public-read authorization model.
func TestAdminTokenOnlyProtectsMutationRoutes(t *testing.T) {
	store := &adminTestStore{}
	handler := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), "upload-secret").Handler()

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/v1/packages", nil))
	if read.Code != http.StatusOK {
		t.Fatalf("anonymous read status: %d", read.Code)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodDelete, "/v1/admin/packages/core/tool/1.0.0", nil))
	if unauthorized.Code != http.StatusUnauthorized || store.removed {
		t.Fatalf("unauthorized removal: status=%d removed=%v", unauthorized.Code, store.removed)
	}

	authorizedRequest := httptest.NewRequest(http.MethodDelete, "/v1/admin/packages/core/tool/1.0.0", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer upload-secret")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent || !store.removed {
		t.Fatalf("authorized removal: status=%d removed=%v", authorized.Code, store.removed)
	}
}

// TestUploadRejectsImperativeProviderCapabilities rejects unsafe package capabilities.
func TestUploadRejectsImperativeProviderCapabilities(t *testing.T) {
	manifest := catalog.PackageVersion{SchemaVersion: 1, Namespace: "core", Name: "tool", Version: "1.0.0", PublishedAt: time.Now(), Signature: catalog.Signature{KeyID: "test", Algorithm: "ed25519", Value: "x"}, SensitiveCapabilities: []string{"local-exec"}}
	if err := validateUpload(manifest, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "local-exec") {
		t.Fatalf("expected local-exec rejection, got %v", err)
	}
}

// TestUploadRejectsInvalidBundle prevents publishing malformed package objects.
func TestUploadRejectsInvalidBundle(t *testing.T) {
	store := &adminTestStore{}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewWithSigning(store, slog.New(slog.NewTextHandler(io.Discard, nil)), "upload-secret", SigningConfig{KeyID: "test", PrivateKey: privateKey}).Handler()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("package", "invalid.cspkg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not a zip")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/packages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer upload-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: %d body=%s", response.Code, response.Body.String())
	}
}

func TestUploadFinalizesAndSignsBundle(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := &adminTestStore{}
	handler := NewWithSigning(store, slog.New(slog.NewTextHandler(io.Discard, nil)), "upload-secret", SigningConfig{KeyID: "central-2026", PrivateKey: privateKey}).Handler()
	var bundle bytes.Buffer
	zipWriter := zip.NewWriter(&bundle)
	files := map[string]string{
		"package.yaml": "schemaVersion: 1\nnamespace: core\nname: tool\nversion: 1.0.0\nsummary: Tool\ndescription: Test tool\npublisher: CodeStation\ncategories: [CLI]\ntargets: [linux-amd64]\ntofuConstraint: '>= 1.8.0'\nmodule: main.tf\nreadme: README.md\nicon: icon.svg\n",
		"main.tf":      "terraform {}\n",
		"README.md":    "# Tool\n",
		"icon.svg":     "<svg/>\n",
	}
	for _, name := range []string{"README.md", "icon.svg", "main.tf", "package.yaml"} {
		part, createErr := zipWriter.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := part.Write([]byte(files[name])); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	part, err := multipartWriter.CreateFormFile("package", "tool.cspkg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bundle.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/packages", &body)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	request.Header.Set("Authorization", "Bearer upload-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	canonical, err := canonicalManifest(store.published)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(store.published.Signature.Value)
	if err != nil || !ed25519.Verify(publicKey, canonical, signature) {
		t.Fatal("registry signature is invalid")
	}
	if bytes.Contains(canonical, []byte(`\u003e`)) || !bytes.Contains(canonical, []byte(`">= 1.8.0"`)) {
		t.Fatalf("canonical JSON uses incompatible HTML escaping: %s", canonical)
	}
}
