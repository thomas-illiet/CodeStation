#!/usr/bin/env sh
set -eu

package_dir=${1:?usage: verify-package.sh PACKAGE_DIRECTORY}

if rg -n 'provisioner[[:space:]]+"|local-exec|remote-exec' "$package_dir"; then
  echo "imperative OpenTofu provisioners are not allowed in CodeStation packages" >&2
  exit 1
fi
test -f "$package_dir/package.yaml"
test -f "$package_dir/.terraform.lock.hcl"
test -f "$package_dir/README.md"

if rg -n -i '(sudo\b|/usr/(local/)?bin|/etc/|/opt/|https?://(github\.com|releases\.hashicorp\.com|registry\.terraform\.io))' "$package_dir"; then
  echo "package contains a forbidden privilege, system path, or Internet reference" >&2
  exit 1
fi

if ! command -v tofu >/dev/null 2>&1; then
  echo "OpenTofu is required for publication validation" >&2
  exit 1
fi

(cd "$package_dir" && tofu fmt -check -recursive && tofu init -backend=false -input=false && tofu validate)
