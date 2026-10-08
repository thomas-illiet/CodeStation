package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

const providerAddress = "codestation.internal/codestation/packages"

// main serves the CodeStation provider over the Terraform plugin protocol.
func main() {
	plugin.Serve(&plugin.ServeOpts{ProviderFunc: provider, ProviderAddr: providerAddress})
}

// provider declares the complete typed resource surface exposed to packages.
func provider() *schema.Provider {
	return &schema.Provider{ResourcesMap: map[string]*schema.Resource{
		"codestation_binary":      resourceArtifact(false),
		"codestation_archive":     resourceArtifact(true),
		"codestation_file":        resourceFile(),
		"codestation_npm_package": resourceNPMPackage(),
	}}
}

// resourceNPMPackage installs a pinned npm package without invoking a shell.
func resourceNPMPackage() *schema.Resource {
	return &schema.Resource{
		CreateContext: func(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
			prefix, err := safeDestination(d.Get("prefix").(string))
			if err != nil {
				return diag.FromErr(err)
			}
			binary := "npm"
			if runtime.GOOS == "windows" {
				binary = "npm.cmd"
			}
			packageVersion := d.Get("package").(string) + "@" + d.Get("version").(string)
			command := exec.CommandContext(ctx, binary, "install", "--global", "--prefix", prefix, packageVersion)
			command.Stdout, command.Stderr = os.Stdout, os.Stderr
			if err := command.Run(); err != nil {
				return diag.Errorf("npm install %s: %v", packageVersion, err)
			}
			d.SetId(packageVersion)
			return nil
		},
		ReadContext: func(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics { return nil },
		DeleteContext: func(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
			prefix, err := safeDestination(d.Get("prefix").(string))
			if err != nil {
				return diag.FromErr(err)
			}
			binary := "npm"
			if runtime.GOOS == "windows" {
				binary = "npm.cmd"
			}
			command := exec.CommandContext(ctx, binary, "uninstall", "--global", "--prefix", prefix, d.Get("package").(string))
			command.Stdout, command.Stderr = os.Stdout, os.Stderr
			if err := command.Run(); err != nil {
				return diag.Errorf("npm uninstall %s: %v", d.Get("package"), err)
			}
			d.SetId("")
			return nil
		},
		Schema: map[string]*schema.Schema{
			"package": {Type: schema.TypeString, Required: true, ForceNew: true},
			"version": {Type: schema.TypeString, Required: true, ForceNew: true},
			"prefix":  {Type: schema.TypeString, Required: true, ForceNew: true},
		},
	}
}

// commonSchema returns fields shared by managed file and archive resources.
func commonSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"version":          {Type: schema.TypeString, Required: true, ForceNew: true},
		"destination":      {Type: schema.TypeString, Required: true, ForceNew: true},
		"urls":             {Type: schema.TypeMap, Required: true, ForceNew: true, Elem: &schema.Schema{Type: schema.TypeString}},
		"sha256s":          {Type: schema.TypeMap, Required: true, ForceNew: true, Elem: &schema.Schema{Type: schema.TypeString}},
		"installed_sha256": {Type: schema.TypeString, Computed: true},
	}
}

// resourceArtifact constructs either a binary download or archive-member resource.
func resourceArtifact(archive bool) *schema.Resource {
	s := commonSchema()
	if archive {
		s["members"] = &schema.Schema{Type: schema.TypeMap, Required: true, ForceNew: true, Elem: &schema.Schema{Type: schema.TypeString}}
	}
	return &schema.Resource{CreateContext: func(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
		return installArtifact(ctx, d, archive)
	}, ReadContext: readManagedFile, DeleteContext: deleteManagedFile, Schema: s}
}

// resourceFile manages deterministic inline content below the user's home directory.
func resourceFile() *schema.Resource {
	return &schema.Resource{
		CreateContext: func(ctx context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
			destination, err := safeDestination(d.Get("destination").(string))
			if err != nil {
				return diag.FromErr(err)
			}
			content := []byte(d.Get("content").(string))
			if err := installBytes(destination, content); err != nil {
				return diag.FromErr(err)
			}
			d.SetId(destination)
			_ = d.Set("installed_sha256", digest(content))
			return nil
		},
		ReadContext: readManagedFile, DeleteContext: deleteManagedFile,
		Schema: map[string]*schema.Schema{
			"version":          {Type: schema.TypeString, Required: true, ForceNew: true},
			"destination":      {Type: schema.TypeString, Required: true, ForceNew: true},
			"content":          {Type: schema.TypeString, Required: true, ForceNew: true},
			"installed_sha256": {Type: schema.TypeString, Computed: true},
		},
	}
}

// installArtifact downloads, verifies and atomically installs an artifact.
func installArtifact(ctx context.Context, d *schema.ResourceData, archive bool) diag.Diagnostics {
	key := runtime.GOOS + "_" + normalizedArch()
	url, ok := stringMap(d.Get("urls"))[key]
	if !ok {
		return diag.Errorf("package version %s does not support %s", d.Get("version"), key)
	}
	expected, ok := stringMap(d.Get("sha256s"))[key]
	if !ok || len(expected) != 64 {
		return diag.Errorf("missing SHA-256 for %s", key)
	}

	body, err := download(ctx, url)
	if err != nil {
		return diag.FromErr(err)
	}
	if actual := digest(body); actual != strings.ToLower(expected) {
		return diag.Errorf("SHA-256 mismatch for %s: expected %s, received %s", url, expected, actual)
	}
	if archive {
		member, ok := stringMap(d.Get("members"))[key]
		if !ok {
			return diag.Errorf("missing archive member for %s", key)
		}
		body, err = extractMember(body, url, member)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	destination, err := safeDestination(d.Get("destination").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	if err := installBytes(destination, body); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(destination)
	_ = d.Set("installed_sha256", digest(body))
	return nil
}

// readManagedFile detects drift without silently replacing modified content.
func readManagedFile(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	if d.Id() == "" {
		return nil
	}
	body, err := os.ReadFile(d.Id())
	if errors.Is(err, os.ErrNotExist) {
		d.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	if digest(body) != d.Get("installed_sha256").(string) {
		d.SetId("")
	}
	return nil
}

// deleteManagedFile removes only content whose installed digest still matches.
func deleteManagedFile(_ context.Context, d *schema.ResourceData, _ interface{}) diag.Diagnostics {
	if d.Id() == "" {
		return nil
	}
	body, err := os.ReadFile(d.Id())
	if errors.Is(err, os.ErrNotExist) {
		d.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	if digest(body) != d.Get("installed_sha256").(string) {
		return diag.Errorf("refusing to remove modified file %s", d.Id())
	}
	if err := os.Remove(d.Id()); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

// download retrieves an HTTPS artifact with bounded size and redirect policy.
func download(ctx context.Context, url string) ([]byte, error) {
	parsed, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if parsed.URL.Scheme != "https" && parsed.URL.Hostname() != "127.0.0.1" && parsed.URL.Hostname() != "localhost" {
		return nil, fmt.Errorf("artifact URL must use HTTPS")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("download %s: %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (512<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 512<<20 {
		return nil, fmt.Errorf("artifact exceeds 512 MiB")
	}
	return body, nil
}

// extractMember returns one regular file from a gzip or plain tar archive.
func extractMember(body []byte, url, member string) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(url), ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if filepath.ToSlash(file.Name) != filepath.ToSlash(member) {
				continue
			}
			input, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer input.Close()
			return io.ReadAll(io.LimitReader(input, 512<<20))
		}
		return nil, fmt.Errorf("archive member %q not found", member)
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.ToSlash(header.Name) == filepath.ToSlash(member) {
			return io.ReadAll(io.LimitReader(tr, 512<<20))
		}
	}
	return nil, fmt.Errorf("archive member %q not found", member)
}

// installBytes writes executable content atomically with owner-only permissions.
func installBytes(destination string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary := destination + ".codestation-new"
	if err := os.WriteFile(temporary, body, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// safeDestination resolves a path and rejects destinations outside the user's home.
func safeDestination(raw string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if raw == "~" {
		raw = home
	} else if strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, "~\\") {
		raw = filepath.Join(home, raw[2:])
	}
	destination, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(home, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("destination must be inside the current user profile")
	}
	return destination, nil
}

// stringMap normalizes Terraform map values into string environment variables.
func stringMap(value interface{}) map[string]string {
	result := map[string]string{}
	for key, item := range value.(map[string]interface{}) {
		result[key] = item.(string)
	}
	return result
}

// normalizedArch maps Go architecture names to package-manifest identifiers.
func normalizedArch() string {
	if runtime.GOARCH == "amd64" {
		return "amd64"
	}
	return runtime.GOARCH
}

// digest returns the lowercase SHA-256 checksum of body.
func digest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
