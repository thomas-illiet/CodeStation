package httpapi

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"codestation/registry/internal/catalog"
	"gopkg.in/yaml.v3"
)

type packageDescriptor struct {
	SchemaVersion         int      `yaml:"schemaVersion"`
	Namespace             string   `yaml:"namespace"`
	Name                  string   `yaml:"name"`
	Version               string   `yaml:"version"`
	Summary               string   `yaml:"summary"`
	Description           string   `yaml:"description"`
	Publisher             string   `yaml:"publisher"`
	Categories            []string `yaml:"categories"`
	Targets               []string `yaml:"targets"`
	TofuConstraint        string   `yaml:"tofuConstraint"`
	Module                string   `yaml:"module"`
	Readme                string   `yaml:"readme"`
	Icon                  string   `yaml:"icon"`
	SensitiveCapabilities []string `yaml:"sensitiveCapabilities,omitempty"`
}

type preparedPackage struct {
	manifest              catalog.PackageVersion
	archive, readme, icon []byte
}

func prepareBundle(bundle []byte, signer SigningConfig, publishedAt time.Time) (preparedPackage, error) {
	if len(signer.PrivateKey) != ed25519.PrivateKeySize || signer.KeyID == "" {
		return preparedPackage{}, errors.New("package signing is not configured")
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		return preparedPackage{}, errors.New("invalid cspkg archive")
	}
	files := map[string][]byte{}
	var total int64
	for _, entry := range reader.File {
		clean := path.Clean(entry.Name)
		if clean != entry.Name || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || entry.FileInfo().Mode()&^0777 != 0 || entry.FileInfo().IsDir() {
			return preparedPackage{}, fmt.Errorf("unsafe cspkg entry %q", entry.Name)
		}
		if _, exists := files[clean]; exists {
			return preparedPackage{}, fmt.Errorf("duplicate cspkg entry %q", clean)
		}
		if entry.UncompressedSize64 > 32<<20 {
			return preparedPackage{}, fmt.Errorf("cspkg entry %q is too large", clean)
		}
		stream, err := entry.Open()
		if err != nil {
			return preparedPackage{}, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, (32<<20)+1))
		stream.Close()
		if err != nil || len(data) > 32<<20 {
			return preparedPackage{}, fmt.Errorf("invalid cspkg entry %q", clean)
		}
		total += int64(len(data))
		if total > 128<<20 {
			return preparedPackage{}, errors.New("expanded cspkg exceeds maximum size")
		}
		files[clean] = data
	}
	var descriptor packageDescriptor
	source, ok := files["package.yaml"]
	if !ok {
		return preparedPackage{}, errors.New("package.yaml is required")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)
	if err := decoder.Decode(&descriptor); err != nil {
		return preparedPackage{}, errors.New("invalid package.yaml")
	}
	if err := validateDescriptor(descriptor); err != nil {
		return preparedPackage{}, err
	}
	if descriptor.Module == "" {
		descriptor.Module = "main.tf"
	}
	if descriptor.Readme == "" {
		descriptor.Readme = "README.md"
	}
	if descriptor.Icon == "" {
		descriptor.Icon = "icon.svg"
	}
	readme, ok := files[descriptor.Readme]
	if !ok {
		return preparedPackage{}, errors.New("declared README is missing")
	}
	icon, ok := files[descriptor.Icon]
	if !ok {
		return preparedPackage{}, errors.New("declared icon is missing")
	}
	if _, ok := files[descriptor.Module]; !ok {
		return preparedPackage{}, errors.New("declared module is missing")
	}
	tfNames := []string{}
	for name := range files {
		if path.Ext(name) == ".tf" {
			text := string(files[name])
			for _, forbidden := range []string{"local-exec", "remote-exec", "provisioner \"", "sudo ", "/etc/", "/opt/", "registry.terraform.io"} {
				if strings.Contains(text, forbidden) {
					return preparedPackage{}, fmt.Errorf("forbidden package content %q in %s", forbidden, name)
				}
			}
			tfNames = append(tfNames, name)
		}
	}
	sort.Strings(tfNames)
	if len(tfNames) == 0 {
		return preparedPackage{}, errors.New("package contains no OpenTofu module")
	}
	archive, refs, err := moduleArchive(tfNames, files)
	if err != nil {
		return preparedPackage{}, err
	}
	prefix := descriptor.Namespace + "/" + descriptor.Name + "/" + descriptor.Version + "/"
	platforms, architectures := targetAxes(descriptor.Targets)
	manifest := catalog.PackageVersion{SchemaVersion: 1, Namespace: descriptor.Namespace, Name: descriptor.Name, Version: descriptor.Version, Summary: descriptor.Summary, Description: descriptor.Description, Publisher: descriptor.Publisher, Categories: descriptor.Categories, Platforms: platforms, Architectures: architectures, Targets: descriptor.Targets, TofuConstraint: descriptor.TofuConstraint, Module: fileRef(descriptor.Module, files[descriptor.Module]), Readme: fileRef(prefix+"README.md", readme), Icon: fileRef(prefix+"icon"+path.Ext(descriptor.Icon), icon), Archive: sizedRef(prefix+"package.tar.gz", archive), Files: refs, Providers: []string{"codestation.internal/codestation/packages"}, SensitiveCapabilities: descriptor.SensitiveCapabilities, PublishedAt: publishedAt.UTC(), Signature: catalog.Signature{KeyID: signer.KeyID, Algorithm: "ed25519"}}
	canonical, err := canonicalManifest(manifest)
	if err != nil {
		return preparedPackage{}, err
	}
	manifest.Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(signer.PrivateKey, canonical))
	return preparedPackage{manifest: manifest, archive: archive, readme: readme, icon: icon}, nil
}

func validateDescriptor(value packageDescriptor) error {
	if value.SchemaVersion != 1 || !packageSegment.MatchString(value.Namespace) || !packageSegment.MatchString(value.Name) || !packageVersion.MatchString(value.Version) {
		return errors.New("package identity is invalid")
	}
	if value.Summary == "" || len(value.Summary) > 140 || value.Description == "" || value.Publisher == "" || value.TofuConstraint == "" {
		return errors.New("package metadata is incomplete")
	}
	allowedTargets := map[string]bool{"linux-amd64": true, "linux-arm64": true, "windows-amd64": true}
	if len(value.Targets) == 0 {
		return errors.New("at least one package target is required")
	}
	for _, target := range value.Targets {
		if !allowedTargets[target] {
			return fmt.Errorf("package target %q is not allowed", target)
		}
	}
	allowedCapabilities := map[string]bool{"external": true, "network": true, "filesystem-outside-xdg": true}
	for _, capability := range value.SensitiveCapabilities {
		if !allowedCapabilities[capability] {
			return fmt.Errorf("sensitive capability %q is not allowed", capability)
		}
	}
	safe := regexp.MustCompile(`^[^/].*$`)
	for _, name := range []string{value.Module, value.Readme, value.Icon} {
		if name != "" && (!safe.MatchString(name) || path.Clean(name) != name || strings.HasPrefix(name, "../")) {
			return fmt.Errorf("unsafe package path %q", name)
		}
	}
	return nil
}

func moduleArchive(names []string, files map[string][]byte) ([]byte, []catalog.SizedFileRef, error) {
	var output bytes.Buffer
	gzipWriter, _ := gzip.NewWriterLevel(&output, gzip.BestCompression)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	tarWriter := tar.NewWriter(gzipWriter)
	refs := make([]catalog.SizedFileRef, 0, len(names))
	for _, name := range names {
		data := files[name]
		header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, nil, err
		}
		if _, err := tarWriter.Write(data); err != nil {
			return nil, nil, err
		}
		ref := sizedRef(name, data)
		refs = append(refs, ref)
	}
	if err := tarWriter.Close(); err != nil {
		return nil, nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, nil, err
	}
	return output.Bytes(), refs, nil
}
func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func fileRef(name string, data []byte) catalog.FileRef {
	return catalog.FileRef{Path: name, SHA256: digestBytes(data)}
}
func sizedRef(name string, data []byte) catalog.SizedFileRef {
	return catalog.SizedFileRef{Path: name, SHA256: digestBytes(data), Size: int64(len(data))}
}
func targetAxes(targets []string) ([]string, []string) {
	platformSet := map[string]bool{}
	architectureSet := map[string]bool{}
	for _, target := range targets {
		parts := strings.SplitN(target, "-", 2)
		platformSet[parts[0]] = true
		architectureSet[parts[1]] = true
	}
	platforms := make([]string, 0, len(platformSet))
	architectures := make([]string, 0, len(architectureSet))
	for item := range platformSet {
		platforms = append(platforms, item)
	}
	for item := range architectureSet {
		architectures = append(architectures, item)
	}
	sort.Strings(platforms)
	sort.Strings(architectures)
	return platforms, architectures
}
func canonicalManifest(manifest catalog.PackageVersion) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	delete(object, "signature")
	if values, ok := object["providers"].([]any); ok && len(values) == 0 {
		delete(object, "providers")
	}
	if values, ok := object["sensitiveCapabilities"].([]any); ok && len(values) == 0 {
		delete(object, "sensitiveCapabilities")
	}
	if retired, ok := object["retired"].(bool); ok && !retired {
		delete(object, "retired")
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(canonical.Bytes(), []byte("\n")), nil
}
