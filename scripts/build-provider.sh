#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=1.0.0
packages_root="$root/providers/codestation.internal/codestation/packages"
output="$packages_root/$version"

rm -rf "$packages_root"
mkdir -p "$packages_root"
for target in linux_amd64 linux_arm64 windows_amd64; do
  os=${target%_*}
  arch=${target#*_}
  directory="$output/$target"
  mkdir -p "$directory"
  printf 'Building CodeStation provider %s for %s/%s\n' "$version" "$os" "$arch"
  docker build \
    --file "$root/provider/Dockerfile" \
    --build-arg TARGETOS="$os" \
    --build-arg TARGETARCH="$arch" \
    --output "type=local,dest=$directory" \
    "$root"
done
