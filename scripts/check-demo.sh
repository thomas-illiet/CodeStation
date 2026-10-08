#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
compose="$root/demo/compose.yaml"
code_server_url="http://127.0.0.1:${CODE_SERVER_PORT:-8080}"
registry_port=${REGISTRY_PORT:-8081}
registry_url="http://127.0.0.1:${registry_port}/v1/packages"
attempt=1

while [ "$attempt" -le 60 ]; do
  if curl --fail --silent --show-error "$code_server_url" >/dev/null 2>&1 && \
    catalog=$(curl --fail --silent --show-error "$registry_url") && \
    printf '%s' "$catalog" | grep -q 'hello-codestation' && \
    docker compose -f "$compose" exec -T code-server sh -lc 'node --version >/dev/null && npm --version >/dev/null'; then
    printf 'Demo ready: %s, npm, and signed package catalog are available\n' "$code_server_url"
    exit 0
  fi

  attempt=$((attempt + 1))
  sleep 2
done

printf 'Demo did not become ready within 120 seconds\n' >&2
docker compose -f "$compose" ps >&2
docker compose -f "$compose" logs --tail=100 >&2
exit 1
