#!/usr/bin/env sh
set -eu

mkdir -p /home/coder/.local/share/code-server/User
# Package operations cannot survive a code-server container restart. Remove the
# previous extension host's lock before installing and starting the extension.
rm -f /home/coder/.local/share/codestation/operation.lock
code-server --uninstall-extension codestation.codestation >/tmp/codestation-extension-uninstall.log 2>&1 || true
code-server --install-extension /opt/codestation/codestation.vsix --force >/tmp/codestation-extension-install.log

cat > /home/coder/.local/share/code-server/User/settings.json <<EOF
{
  "codestation.registryUrl": "${CODESTATION_REGISTRY_URL:-http://registry:8080}",
  "codestation.logRetentionDays": 30,
  "codestation.maxLogSizeMiB": 20,
  "workbench.startupEditor": "readme",
  "security.workspace.trust.enabled": false,
  "telemetry.telemetryLevel": "off",
  "extensions.autoUpdate": false,
  "extensions.autoCheckUpdates": false
}
EOF

exec "$@"
