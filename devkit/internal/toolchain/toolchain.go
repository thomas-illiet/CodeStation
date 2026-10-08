package toolchain

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed assets
var assets embed.FS

type Runtime struct {
	Tofu       string
	Mirror     string
	ConfigFile string
}

func Prepare() (Runtime, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return Runtime{}, err
	}
	root := filepath.Join(cache, "codestation", "devkit", "1.0.0", runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(root, 0700); err != nil {
		return Runtime{}, err
	}
	tofuName := "tofu"
	providerName := "terraform-provider-packages_v1.0.0"
	if runtime.GOOS == "windows" {
		tofuName += ".exe"
		providerName += ".exe"
	}
	tofu, tofuEmbedded, err := materialize("assets/tofu", filepath.Join(root, tofuName))
	if err != nil {
		return Runtime{}, err
	}
	provider, providerEmbedded, err := materialize("assets/provider", filepath.Join(root, "providers", "codestation.internal", "codestation", "packages", "1.0.0", runtime.GOOS+"_"+runtime.GOARCH, providerName))
	if err != nil {
		return Runtime{}, err
	}
	if !tofuEmbedded {
		tofu = os.Getenv("CODESTATION_TOFU")
		if tofu == "" {
			tofu, err = exec.LookPath(tofuName)
		}
		if err != nil || tofu == "" {
			return Runtime{}, errors.New("this development build has no embedded OpenTofu; set CODESTATION_TOFU or use a release binary")
		}
	}
	mirror := filepath.Join(root, "providers")
	if !providerEmbedded {
		mirror = os.Getenv("CODESTATION_PROVIDER_MIRROR")
		if mirror == "" {
			return Runtime{}, errors.New("this development build has no embedded provider; set CODESTATION_PROVIDER_MIRROR or use a release binary")
		}
	}
	config := filepath.Join(root, "tofurc")
	content := fmt.Sprintf("provider_installation {\n  filesystem_mirror {\n    path = %q\n    include = [\"codestation.internal/codestation/packages\"]\n  }\n}\n", filepath.ToSlash(mirror))
	if err := os.WriteFile(config, []byte(content), 0600); err != nil {
		return Runtime{}, err
	}
	_ = provider
	return Runtime{Tofu: tofu, Mirror: mirror, ConfigFile: config}, nil
}

func materialize(name, destination string) (string, bool, error) {
	data, err := fs.ReadFile(assets, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if len(data) == 0 {
		return "", false, nil
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if current, readErr := os.ReadFile(destination); readErr == nil {
		currentSum := sha256.Sum256(current)
		if hex.EncodeToString(currentSum[:]) == checksum {
			return destination, true, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return "", false, err
	}
	temporary := destination + ".tmp"
	if err := os.WriteFile(temporary, data, 0700); err != nil {
		return "", false, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return "", false, err
	}
	return destination, true, nil
}
