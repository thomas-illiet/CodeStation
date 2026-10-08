#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=1.0.0
assets="$root/devkit/internal/toolchain/assets"
output="$root/dist/devkit"
mkdir -p "$output"

cleanup() { rm -f "$assets/tofu" "$assets/provider"; }
trap cleanup EXIT INT TERM

for target in linux_amd64 linux_arm64 windows_amd64; do
  os=${target%_*}; arch=${target#*_}; suffix=""; binary=tofu
  [ "$os" = windows ] && suffix=.exe && binary=tofu.exe
  cp "$root/bin/$os-$([ "$arch" = amd64 ] && printf x64 || printf %s "$arch")/$binary" "$assets/tofu"
  provider_dir="$root/providers/codestation.internal/codestation/packages/$version/${os}_${arch}"
  provider=$(find "$provider_dir" -type f -name 'terraform-provider-*' | head -1)
  cp "$provider" "$assets/provider"
  (cd "$root/devkit" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$output/codestation_${version}_${os}_${arch}${suffix}" ./cmd/codestation)
  cleanup
done

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$output" && sha256sum codestation_* > SHA256SUMS)
else
  (cd "$output" && shasum -a 256 codestation_* > SHA256SUMS)
fi
