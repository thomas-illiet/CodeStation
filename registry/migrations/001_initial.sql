BEGIN;

CREATE TABLE IF NOT EXISTS package_versions (
    namespace TEXT NOT NULL CHECK (namespace ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    name TEXT NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    version TEXT NOT NULL,
    manifest JSONB NOT NULL,
    retired BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (namespace, name, version),
    CHECK (manifest->>'namespace' = namespace),
    CHECK (manifest->>'name' = name),
    CHECK (manifest->>'version' = version)
);

CREATE INDEX IF NOT EXISTS package_versions_latest_idx ON package_versions (namespace, name, published_at DESC) WHERE retired = FALSE;
CREATE INDEX IF NOT EXISTS package_versions_categories_idx ON package_versions USING GIN ((manifest->'categories'));

COMMIT;
