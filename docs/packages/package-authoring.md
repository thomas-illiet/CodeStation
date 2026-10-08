# Package authoring

Install a released `codestation` dev-kit binary or use the multi-architecture
dev-kit image. VS Code is not required. Start a package with:

```bash
codestation package init ./my-tool \
  --namespace my-team --name my-tool --version 1.0.0 \
  --summary "Install my tool" --publisher "My team"
```

The generated `package.yaml` is the author-owned metadata contract. The final
signed JSON manifest is produced by the registry and must not be maintained by
authors.

Authors must:

- pin the bundled `codestation.internal/codestation/packages` provider in `.terraform.lock.hcl`;
- use only its typed `codestation_binary`, `codestation_archive`, `codestation_file` and `codestation_npm_package` resources;
- avoid `sudo`, setuid files and system paths;
- make apply and destroy idempotent; arbitrary provisioners and shell commands are rejected;
- use only the typed resources exposed by the CodeStation provider;
- declare `network` and use HTTPS endpoints approved by the publication policy; every downloaded artifact must have a platform-specific pinned SHA-256 digest.

Run OpenTofu directly through the embedded, pinned toolchain. Every argument,
prompt, stream and exit status is passed through unchanged:

```bash
codestation package --chdir ./my-tool plan -out install.tfplan
codestation package --chdir ./my-tool apply install.tfplan
codestation package --chdir ./my-tool destroy
```

Upload validates the sources, creates a deterministic temporary `.cspkg`, and
sends it to the registry. The registry recomputes all digests and signs the
canonical manifest. Published versions are immutable.

```bash
export CODESTATION_ADMIN_TOKEN='...'
codestation package upload ./my-tool --registry https://packages.example.test
codestation package delete my-team/my-tool@1.0.0 \
  --registry https://packages.example.test --yes
```

`--token` overrides `CODESTATION_ADMIN_TOKEN` on upload and delete, but a
literal token may be visible in shell history and process listings. Prefer the
environment variable in CI. The CLI never prints the token.

The administration token is scoped to upload and deletion. Catalogue and
download requests remain anonymous.
