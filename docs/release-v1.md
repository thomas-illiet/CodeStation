# Version 1 release checklist

## Automated gate

Run from a clean checkout:

```bash
npm ci
make release-check
make package
```

`make package` builds the provider for Linux AMD64, Linux ARM64 and Windows
AMD64 before producing `codestation-1.0.0.vsix`. The required OpenTofu 1.13.1
binaries are downloaded by Make, checked against pinned SHA-256 digests and are
never committed.

## Container validation

```bash
docker build -f demo/code-server.Dockerfile -t codestation-code-server:1.0.0 .
make demo-up
```

Install, execute and remove the Hello package from the Packages view. Confirm
that `make demo-seed` completed and that the registry and code-server
containers are healthy, then run `make demo-down`.

## Publication

1. Confirm `git status` contains only the intended release files.
2. Review `CHANGELOG.md` and the platform matrix.
3. Create the first commit without generated keys, `.env` files, VSIX files or provider build outputs.
4. Tag the reviewed commit as `v1.0.0`.
5. Publish the VSIX and container images from that immutable tag.
6. Record SHA-256 digests for every published artifact.
