#!/bin/sh
set -eu

: "${CODESTATION_REGISTRY_URL:?set CODESTATION_REGISTRY_URL}"
: "${CODESTATION_ADMIN_TOKEN:?set CODESTATION_ADMIN_TOKEN}"

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

attempt=1
until curl --fail --silent --output /dev/null "${CODESTATION_REGISTRY_URL%/}/readyz"; do
  if [ "$attempt" -ge 30 ]; then
    echo "registry did not become ready" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 1
done

container_registry=$(printf '%s' "$CODESTATION_REGISTRY_URL" | sed 's/127\.0\.0\.1/host.docker.internal/;s/localhost/host.docker.internal/')
for package_dir in "$root"/examples/packages/*; do
  identity=$(basename "$package_dir")
  namespace=$(awk '$1 == "namespace:" { print $2; exit }' "$package_dir/package.yaml")
  name=$(awk '$1 == "name:" { print $2; exit }' "$package_dir/package.yaml")
  version=$(awk '$1 == "version:" { print $2; exit }' "$package_dir/package.yaml")
  if curl --fail --silent --output /dev/null "${CODESTATION_REGISTRY_URL%/}/v1/packages/$namespace/$name/$version"; then
    echo "Already published $namespace/$name@$version"
    continue
  fi
  docker run --rm \
    --add-host=host.docker.internal:host-gateway \
    -e CODESTATION_ADMIN_TOKEN \
    -v "$root/examples/packages:/packages:ro" \
    codestation-devkit:local \
    package upload "/packages/$identity" --registry "$container_registry"
done
