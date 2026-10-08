#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=1.13.1
base_url="https://github.com/opentofu/opentofu/releases/download/v${version}"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

download() {
  platform=$1
  destination=$2
  executable=$3
  expected=$4
  output="$root/bin/$destination/$executable"

  if [ -f "$output" ] && [ "$(digest "$output")" = "$expected" ]; then
    printf 'OpenTofu %s for %s is already available\n' "$version" "$platform"
    return
  fi

  archive="$temporary/tofu_${version}_${platform}.zip"
  directory="$temporary/$destination"
  mkdir -p "$directory" "$(dirname "$output")"
  printf 'Downloading OpenTofu %s for %s\n' "$version" "$platform"
  curl --fail --location --silent --show-error \
    --output "$archive" \
    "$base_url/tofu_${version}_${platform}.zip"
  unzip -q "$archive" "$executable" -d "$directory"

  actual=$(digest "$directory/$executable")
  if [ "$actual" != "$expected" ]; then
    printf 'Checksum mismatch for %s: expected %s, got %s\n' "$platform" "$expected" "$actual" >&2
    exit 1
  fi

  mv "$directory/$executable" "$output"
  chmod 0755 "$output"
}

download linux_amd64 linux-x64 tofu a325c8c2f6834575e440b03c2ba67f94256072754ac7787fea718be6f01fef6a
download linux_arm64 linux-arm64 tofu d5c690023baebe8bf2cfb2062e292186f4797a173db22c973bd9de45cb68a62f
download windows_amd64 windows-x64 tofu.exe 554ec8acba937e516c5b6ba93ee46172420f6365e8dfe41e70218157cd4a802d
