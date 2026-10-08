# Architecture

## Trust and data flow

1. The standalone dev kit validates a package against its embedded CodeStation provider and produces a deterministic `.cspkg` upload bundle.
2. The registry revalidates the bundle, builds the immutable module archive and signs the canonical manifest with Ed25519.
3. The extension reads only the anonymous catalogue API. It downloads an immutable archive, verifies size, SHA-256 and signature, then extracts it with path and symlink protections.
4. The embedded OpenTofu process loads the signed, bundled CodeStation provider from a filesystem mirror and never contacts a public provider registry. Output is streamed as JSON to the installation webview and redacted before local persistence.
5. Each package has an independent state directory. A cross-window file lock and an in-process queue permit one mutation at a time.

The registry never receives local state, logs, machine identifiers or installation events.

## Local paths

| Data | Default |
| --- | --- |
| Package states | `$XDG_DATA_HOME/codestation/installations` or `~/.local/share/codestation/installations` |
| Catalogue cache | `$XDG_CACHE_HOME/codestation` or `~/.cache/codestation` |
| Logs | `$XDG_STATE_HOME/codestation/logs` or `~/.local/state/codestation/logs` |

Directories use mode `0700`; state, registry metadata and logs use `0600`. State is copied atomically before an operation. A failed destroy remains installed with an error status.

## Deployment

The registry configures metadata and object storage independently. Set `CODESTATION_METADATA_PROVIDER` to `sqlite` or `postgres`, and set `CODESTATION_OBJECT_STORAGE_PROVIDER` to `local` or `s3`. All four combinations are supported: SQLite/local, SQLite/S3, PostgreSQL/local, and PostgreSQL/S3.

SQLite is initialized automatically at `CODESTATION_SQLITE_PATH` (default `<data>/registry.db`). PostgreSQL uses `CODESTATION_DATABASE_URL` and requires `registry/migrations/001_initial.sql` to be applied. Local objects are written beneath `CODESTATION_LOCAL_OBJECTS_PATH` (default `<data>/objects`; the older `CODESTATION_OBJECTS_PATH` remains accepted). S3-compatible storage uses the `CODESTATION_S3_ENDPOINT`, `CODESTATION_S3_ACCESS_KEY`, `CODESTATION_S3_SECRET_KEY`, `CODESTATION_S3_BUCKET`, `CODESTATION_S3_USE_TLS`, and optional `CODESTATION_S3_REGION` settings.

If only one new provider setting is supplied, metadata defaults to PostgreSQL when `CODESTATION_DATABASE_URL` exists and SQLite otherwise; object storage defaults to S3 when `CODESTATION_S3_ENDPOINT` exists and local otherwise. With neither new setting, the historical modes remain compatible: `CODESTATION_DATABASE_PROVIDER=sqlite` maps to SQLite/local, `postgres` maps to PostgreSQL/S3, and `CODESTATION_DATABASE_URL` alone maps to PostgreSQL/S3. With no storage configuration at all, the read-only `catalog.json` development adapter remains active.

SQLite and local object storage require a writable persistent volume. A SQLite deployment must use a single registry replica. PostgreSQL metadata and S3 object storage can be shared by multiple replicas; local object storage can only be shared when the mounted filesystem itself supports the chosen access mode.

For any writable combination, set `CODESTATION_ADMIN_TOKEN`. The administrator bearer token is accepted only by `POST /v1/admin/packages` and `DELETE /v1/admin/packages/{namespace}/{name}/{version}`; every catalogue and download endpoint remains anonymous. The Helm chart provisions probes and a default-deny NetworkPolicy; PostgreSQL and S3 providers must explicitly receive the required egress rules.

## Deliberate V1 limits

- Linux on `amd64` and `arm64`; Windows on `amd64`.
- User-scoped operations only; modules requiring root are rejected before publication.
- No ratings, payments, telemetry or central inventory.
- Closing the progress view never terminates an operation.
