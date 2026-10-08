package packagefile

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Descriptor struct {
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

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
var version = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

func Load(root string) (Descriptor, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.yaml"))
	if err != nil {
		return Descriptor{}, err
	}
	var descriptor Descriptor
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("package.yaml: %w", err)
	}
	if descriptor.SchemaVersion != 1 || !identifier.MatchString(descriptor.Namespace) || !identifier.MatchString(descriptor.Name) || !version.MatchString(descriptor.Version) {
		return Descriptor{}, errors.New("package identity is invalid")
	}
	if descriptor.Summary == "" || descriptor.Description == "" || descriptor.Publisher == "" || descriptor.TofuConstraint == "" {
		return Descriptor{}, errors.New("summary, description, publisher and tofuConstraint are required")
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
	if len(descriptor.Targets) == 0 {
		return Descriptor{}, errors.New("at least one target is required")
	}
	allowedTargets := map[string]bool{"linux-amd64": true, "linux-arm64": true, "windows-amd64": true}
	for _, target := range descriptor.Targets {
		if !allowedTargets[target] {
			return Descriptor{}, fmt.Errorf("unsupported target %q", target)
		}
	}
	allowedCapabilities := map[string]bool{"external": true, "network": true, "filesystem-outside-xdg": true}
	for _, capability := range descriptor.SensitiveCapabilities {
		if !allowedCapabilities[capability] {
			return Descriptor{}, fmt.Errorf("unsupported capability %q", capability)
		}
	}
	for _, path := range []string{descriptor.Module, descriptor.Readme, descriptor.Icon} {
		if _, err := safePath(root, path); err != nil {
			return Descriptor{}, err
		}
	}
	return descriptor, nil
}

func ValidateSources(root string, descriptor Descriptor) error {
	lock := filepath.Join(root, ".terraform.lock.hcl")
	if _, err := os.Stat(lock); err != nil {
		return errors.New(".terraform.lock.hcl is required")
	}
	for _, required := range []string{descriptor.Module, descriptor.Readme, descriptor.Icon} {
		file, err := safePath(root, required)
		if err != nil {
			return err
		}
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", required)
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are forbidden: %s", path)
		}
		if filepath.Ext(path) != ".tf" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range []string{"local-exec", "remote-exec", "provisioner \"", "sudo ", "/etc/", "/opt/", "registry.terraform.io"} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("forbidden package content %q in %s", forbidden, path)
			}
		}
		return nil
	})
}

func Build(root string) ([]byte, Descriptor, error) {
	descriptor, err := Load(root)
	if err != nil {
		return nil, Descriptor{}, err
	}
	if err := ValidateSources(root, descriptor); err != nil {
		return nil, Descriptor{}, err
	}
	files := []string{"package.yaml", descriptor.Readme, descriptor.Icon}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if filepath.Ext(rel) == ".tf" {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, Descriptor{}, err
	}
	sort.Strings(files)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	epoch := time.Unix(0, 0).UTC()
	for _, name := range files {
		source, err := safePath(root, name)
		if err != nil {
			return nil, Descriptor{}, err
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, Descriptor{}, err
		}
		header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
		header.SetModTime(epoch)
		header.SetMode(0600)
		part, err := writer.CreateHeader(header)
		if err != nil {
			return nil, Descriptor{}, err
		}
		if _, err = part.Write(data); err != nil {
			return nil, Descriptor{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, Descriptor{}, err
	}
	return output.Bytes(), descriptor, nil
}

func safePath(root, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("unsafe package path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe package path %q", name)
	}
	return filepath.Join(root, clean), nil
}

func WriteDeterministic(w io.Writer, data []byte) error { _, err := w.Write(data); return err }
