# CodeStation Packages complete demo

This stack demonstrates the full offline runtime path:

- code-server with the CodeStation Packages extension and an embedded OpenTofu binary;
- an HTTP registry;
- SQLite catalogue metadata;
- immutable package objects stored on the local registry volume;
- a standalone dev-kit image that validates and uploads packages maintained
  under `examples/packages/`;
- central manifest finalization and signing inside the registry.

The image build downloads pinned upstream dependencies. Once images have been built, the running stack does not require Internet access. The registry keeps its SQLite database and package objects in the `registry-data` volume.

## Start

```bash
make demo-up
```

`demo-up` builds every image, starts the stack and waits until code-server and
the signed HTTP catalog are reachable. The local admin token defaults to
`codestation-demo-admin`; override it with `make demo-up
DEMO_ADMIN_TOKEN=another-local-value`.

Useful lifecycle commands:

```bash
make demo-build
make demo-seed
make demo-check
make demo-status
make demo-logs
make demo-down
make demo-clean
make demo-reset
```

`make demo-seed` starts the registry when needed, then runs the external
`scripts/seed-demo.sh` publisher against the authenticated administration API.
The script uses the same `codestation package upload` command available to
package authors.
The Go registry binary only serves the API: it contains no seed command. The
Compose stack has no seed service and never publishes demo packages implicitly.

`demo-down` preserves volumes and disposable keys for a later restart.
`demo-reset` removes volumes and generated key material; `demo-clean` removes
only the generated keys.

Open <http://localhost:8080>, then select the CodeStation icon in the Activity Bar and open **Packages**. Open **Hello CodeStation Packages** and choose **Install**. The installation window streams every OpenTofu phase. In a code-server terminal, run:

```bash
~/.local/bin/codestation-hello
```

Return to the package page and choose **Uninstall** to remove it.

The registry API is exposed directly over HTTP on port `8081`; inspect it from the host with:

```bash
curl http://127.0.0.1:8081/v1/packages
```

## Reset

```bash
make demo-reset
```

## Security warning

`make demo-up` generates disposable signing keys under `demo/keys/`.
They are ignored by Git and must never be reused outside this demo. Passwords in
the Compose file are fixed local-demo values and are not suitable for any other
environment.

All published ports bind explicitly to `127.0.0.1`; do not change that binding on an untrusted network.
