package httpapi

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"codestation/registry/internal/catalog"
)

type Server struct {
	store      catalog.Store
	logger     *slog.Logger
	mux        *http.ServeMux
	adminToken string
	signing    SigningConfig
}

type SigningConfig struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
}

// New creates an HTTP API and registers all catalog and module routes.
func New(store catalog.Store, logger *slog.Logger, adminToken ...string) *Server {
	token := ""
	if len(adminToken) > 0 {
		token = strings.TrimSpace(adminToken[0])
	}
	s := &Server{store: store, logger: logger, mux: http.NewServeMux(), adminToken: token}
	s.routes()
	return s
}

// NewWithSigning creates a writable API that finalizes and signs uploaded cspkg bundles.
func NewWithSigning(store catalog.Store, logger *slog.Logger, adminToken string, signing SigningConfig) *Server {
	s := &Server{store: store, logger: logger, mux: http.NewServeMux(), adminToken: strings.TrimSpace(adminToken), signing: signing}
	s.routes()
	return s
}

// Handler returns the API wrapped with request logging and security headers.
func (s *Server) Handler() http.Handler { return securityHeaders(s.logRequests(s.mux)) }

// routes binds public reads and authenticated mutation endpoints.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("GET /v1/packages", s.list)
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}", s.detail)
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}/versions", s.versions)
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}/{version}", s.version)
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}/{version}/download", s.download)
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}/{version}/readme", s.object("readme"))
	s.mux.HandleFunc("GET /v1/packages/{namespace}/{name}/{version}/icon", s.object("icon"))
	s.mux.HandleFunc("GET /v1/modules/{namespace}/{name}/codestation/versions", s.moduleVersions)
	s.mux.HandleFunc("GET /v1/modules/{namespace}/{name}/codestation/{version}/download", s.moduleDownload)
	s.mux.HandleFunc("GET /v1/providers/{path...}", s.providerObject)
	s.mux.HandleFunc("POST /v1/admin/packages", s.authorize(s.upload))
	s.mux.HandleFunc("DELETE /v1/admin/packages/{namespace}/{name}/{version}", s.authorize(s.remove))
}

const maxUploadSize = 536870912 + 8<<20

var packageSegment = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
var packageVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// authorize validates the bearer token used for catalog mutations.
func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			problem(w, http.StatusServiceUnavailable, "package administration is disabled")
			return
		}
		prefix := "Bearer "
		provided := r.Header.Get("Authorization")
		if !strings.HasPrefix(provided, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(provided, prefix)), []byte(s.adminToken)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			problem(w, http.StatusUnauthorized, "invalid administration token")
			return
		}
		next(w, r)
	}
}

// upload validates one source bundle, finalizes its manifest and signs it centrally.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.store.(catalog.AdminStore)
	if !ok {
		problem(w, http.StatusServiceUnavailable, "catalogue storage is read-only")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		problem(w, http.StatusBadRequest, "invalid multipart package upload")
		return
	}
	defer r.MultipartForm.RemoveAll()
	bundle, err := multipartValue(r.MultipartForm, "package", 536870912)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	prepared, err := prepareBundle(bundle, s.signing, time.Now())
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	manifest := prepared.manifest
	if err := validateUpload(manifest, prepared.archive, prepared.readme, prepared.icon); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := admin.Publish(manifest, prepared.archive, prepared.readme, prepared.icon); errors.Is(err, catalog.ErrConflict) {
		problem(w, http.StatusConflict, "package version already exists")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/v1/packages/%s/%s/%s", manifest.Namespace, manifest.Name, manifest.Version))
	writeJSON(w, http.StatusCreated, manifest)
}

// remove deletes one package release through an AdminStore.
func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.store.(catalog.AdminStore)
	if !ok {
		problem(w, http.StatusServiceUnavailable, "catalogue storage is read-only")
		return
	}
	if err := admin.Remove(r.PathValue("namespace"), r.PathValue("name"), r.PathValue("version")); errors.Is(err, catalog.ErrNotFound) {
		problem(w, http.StatusNotFound, "package version not found")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// multipartValue reads a named form part while enforcing a byte limit.
func multipartValue(form *multipart.Form, name string, limit int64) ([]byte, error) {
	files := form.File[name]
	if len(files) != 1 {
		return nil, fmt.Errorf("exactly one %s file is required", name)
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds maximum size", name)
	}
	return data, nil
}

// validateUpload checks metadata, capabilities, sizes and content digests.
func validateUpload(manifest catalog.PackageVersion, archive, readme, icon []byte) error {
	if manifest.SchemaVersion != 1 || !packageSegment.MatchString(manifest.Namespace) || !packageSegment.MatchString(manifest.Name) || !packageVersion.MatchString(manifest.Version) || manifest.PublishedAt.IsZero() {
		return errors.New("manifest identity is invalid")
	}
	if manifest.Signature.KeyID == "" || manifest.Signature.Algorithm != "ed25519" || manifest.Signature.Value == "" {
		return errors.New("manifest signature metadata is invalid")
	}
	for _, provider := range manifest.Providers {
		if provider != "codestation.internal/codestation/packages" {
			return fmt.Errorf("provider %q is not allowed", provider)
		}
	}
	allowedCapabilities := map[string]bool{"external": true, "network": true, "filesystem-outside-xdg": true}
	for _, capability := range manifest.SensitiveCapabilities {
		if !allowedCapabilities[capability] {
			return fmt.Errorf("sensitive capability %q is not allowed", capability)
		}
	}
	allowedTargets := map[string]bool{"linux-amd64": true, "linux-arm64": true, "windows-amd64": true}
	if len(manifest.Targets) == 0 {
		return errors.New("at least one package target is required")
	}
	for _, target := range manifest.Targets {
		if !allowedTargets[target] {
			return fmt.Errorf("package target %q is not allowed", target)
		}
	}
	prefix := manifest.Namespace + "/" + manifest.Name + "/" + manifest.Version + "/"
	for _, objectPath := range []string{manifest.Archive.Path, manifest.Readme.Path, manifest.Icon.Path} {
		if path.Clean(objectPath) != objectPath || !strings.HasPrefix(objectPath, prefix) {
			return errors.New("manifest object paths must use the package version prefix")
		}
	}
	checks := []struct {
		name, expected string
		size           int64
		data           []byte
	}{
		{"archive", manifest.Archive.SHA256, manifest.Archive.Size, archive},
		{"readme", manifest.Readme.SHA256, int64(len(readme)), readme},
		{"icon", manifest.Icon.SHA256, int64(len(icon)), icon},
	}
	for _, check := range checks {
		hash := sha256.Sum256(check.data)
		if hex.EncodeToString(hash[:]) != check.expected || (check.name == "archive" && int64(len(check.data)) != check.size) {
			return fmt.Errorf("%s digest or size mismatch", check.name)
		}
	}
	return nil
}

// providerObject serves embedded provider binaries by version and target.
func (s *Server) providerObject(w http.ResponseWriter, r *http.Request) {
	objectPath := "providers/" + r.PathValue("path")
	data, contentType, err := s.store.ReadObject(objectPath)
	if errors.Is(err, catalog.ErrNotFound) {
		problem(w, http.StatusNotFound, "provider mirror object not found")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	hash := sha256.Sum256(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", fmt.Sprintf("\"sha256:%s\"", hex.EncodeToString(hash[:])))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, objectPath, time.Unix(0, 0), strings.NewReader(string(data)))
}

// ready reports whether the registry process can serve requests.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	_, err := s.store.List("", "", "name", 1, 1)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "catalog unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// list serves the filtered and paginated package catalog.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	page := boundedInt(r.URL.Query().Get("page"), 1, 1, 100000)
	size := boundedInt(r.URL.Query().Get("pageSize"), 24, 1, 100)
	result, err := s.store.List(r.URL.Query().Get("q"), r.URL.Query().Get("category"), r.URL.Query().Get("sort"), page, size)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// detail resolves the latest manifest for a package.
func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Get(r.PathValue("namespace"), r.PathValue("name"), "latest")
	s.writeItem(w, item, err)
}

// version resolves an explicitly requested package version.
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Get(r.PathValue("namespace"), r.PathValue("name"), r.PathValue("version"))
	s.writeItem(w, item, err)
}

// writeItem maps catalog lookup results into consistent HTTP responses.
func (s *Server) writeItem(w http.ResponseWriter, item catalog.PackageVersion, err error) {
	if errors.Is(err, catalog.ErrNotFound) {
		problem(w, 404, "package not found")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, item)
}

// versions lists the published versions available for one package.
func (s *Server) versions(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Versions(r.PathValue("namespace"), r.PathValue("name"))
	if errors.Is(err, catalog.ErrNotFound) {
		problem(w, 404, "package not found")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"versions": items})
}

// object creates a handler for README and icon objects.
func (s *Server) object(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := s.store.Get(r.PathValue("namespace"), r.PathValue("name"), r.PathValue("version"))
		if err != nil {
			s.writeItem(w, item, err)
			return
		}
		ref := item.Icon
		if kind == "readme" {
			ref = item.Readme
		}
		s.serveObject(w, r, ref)
	}
}

// download streams the immutable archive for a package release.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Get(r.PathValue("namespace"), r.PathValue("name"), r.PathValue("version"))
	if err != nil {
		s.writeItem(w, item, err)
		return
	}
	s.serveObject(w, r, catalog.FileRef{Path: item.Archive.Path, SHA256: item.Archive.SHA256})
}

// serveObject writes a referenced object with its recorded content type.
func (s *Server) serveObject(w http.ResponseWriter, r *http.Request, ref catalog.FileRef) {
	b, ct, err := s.store.ReadObject(ref.Path)
	if errors.Is(err, catalog.ErrNotFound) {
		problem(w, 404, "object not found")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	h := sha256.Sum256(b)
	digest := hex.EncodeToString(h[:])
	if ref.SHA256 != "" && digest != ref.SHA256 {
		problem(w, 500, "stored object failed integrity check")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("ETag", fmt.Sprintf("\"sha256:%s\"", digest))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, ref.Path, time.Unix(0, 0), strings.NewReader(string(b)))
}

// moduleVersions implements the OpenTofu module registry versions protocol.
func (s *Server) moduleVersions(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Versions(r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		s.writeItem(w, catalog.PackageVersion{}, err)
		return
	}
	versions := make([]map[string]string, 0, len(items))
	for _, v := range items {
		if !v.Retired {
			versions = append(versions, map[string]string{"version": v.Version})
		}
	}
	writeJSON(w, 200, map[string]any{"modules": []any{map[string]any{"versions": versions}}})
}

// moduleDownload returns the module archive URL through the protocol header.
func (s *Server) moduleDownload(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.Get(r.PathValue("namespace"), r.PathValue("name"), r.PathValue("version"))
	if err != nil {
		s.writeItem(w, item, err)
		return
	}
	w.Header().Set("X-Terraform-Get", fmt.Sprintf("/v1/packages/%s/%s/%s/download", item.Namespace, item.Name, item.Version))
	w.WriteHeader(http.StatusNoContent)
}

// internal logs a server-side error and emits a generic problem response.
func (s *Server) internal(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "error", err)
	problem(w, 500, "internal server error")
}

// logRequests records method, path, status and duration without request bodies.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// securityHeaders adds defensive headers to every registry response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// writeJSON serializes a successful or structured error response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// problem writes a minimal problem-details response.
func problem(w http.ResponseWriter, status int, title string) {
	writeJSON(w, status, map[string]any{"type": "about:blank", "title": title, "status": status})
}

// boundedInt parses an integer and clamps it to an accepted range.
func boundedInt(raw string, fallback, min, max int) int {
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
