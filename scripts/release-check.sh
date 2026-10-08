#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

test "$(node -p "require('./package.json').version")" = "1.0.0"
test "$(node -p "require('./package-lock.json').version")" = "1.0.0"
grep -q '^version: 1.0.0$' deploy/helm/codestation-registry/Chart.yaml
grep -q '^appVersion: "1.0.0"$' deploy/helm/codestation-registry/Chart.yaml
grep -q '^version=1.0.0$' scripts/build-provider.sh

if rg -n -i 'appmarket' --glob '!CHANGELOG.md' --glob '!scripts/release-check.sh' .; then
  echo 'obsolete product name found' >&2
  exit 1
fi
if rg -n 'provisioner[[:space:]]+"' examples/packages; then
  echo 'imperative package provisioner found' >&2
  exit 1
fi
if rg -n --hidden --glob '!node_modules/**' --glob '!scripts/release-check.sh' -- '-----BEGIN .*PRIVATE KEY-----' .; then
  echo 'private key material found in the repository' >&2
  exit 1
fi

./scripts/download-bins.sh

npm run check
npm test
npm audit --omit=dev
(cd provider && go test ./... && go vet ./...)
(cd registry && go test ./... && go vet ./...)
(cd devkit && go test ./... && go vet ./...)
npm run build
