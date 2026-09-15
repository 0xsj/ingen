# Storage schema versions should be separate from the Amber wire version

Persistence containers and provenance values evolve at different boundaries,
so they should not share one ambiguous version field.

## Origin

The file-backed Go store used `amber.Version` for its snapshot envelope, even
though that value identifies the serialized provenance contract inside the
snapshot. PostgreSQL had an idempotent setup schema but no metadata that could
tell the adapter whether an application-managed migration had been applied.

## What

The Go storage layer now exposes separate version boundaries:

- `FileStoreSchemaVersion` identifies the JSON snapshot format while retaining
  compatibility with the existing `{"version":1,"records":{...}}` envelope.
- `PostgresSchemaVersion` and `PostgresSchemaName` identify the SQL schema
  expected by the PostgreSQL adapter.
- `EnsureSchema` creates the metadata row when needed, then verifies its
  version before the adapter serves data. A mismatch returns
  `ErrUnsupportedSchemaVersion` so the application can migrate deliberately.
- `CheckSchema` performs the same PostgreSQL version verification without
  creating or altering tables, which gives applications a read-only readiness
  check after migrations.

The Amber wire `version` remains the version of each provenance value. The
generic key-value seam does not impose a container schema; the supplied backend
and namespace own that persistence format.

## Why

Wire compatibility and persistence compatibility are related but not
identical. Separating them prevents a future provenance wire version from
silently being treated as a file or SQL migration, and it gives production
deployments a clear point at which to run and verify migrations.

## Gotchas

- `PostgresSchema` is still an idempotent bootstrap schema, not a general
  migration engine.
- `CheckSchema` is read-only; it does not repair a missing or old schema.
- `make postgres-schema-check` is a runnable operator check that uses
  `CheckSchema` without creating tables or writing provenance.
- A deployment must apply any future schema migration before calling
  `EnsureSchema` with the corresponding adapter version.
- Existing PostgreSQL installations created before metadata was introduced
  should run the bootstrap schema once so the metadata row is present.
- The TypeScript key-value contract intentionally leaves storage-container
  versioning to the backend implementation.

## Used in

- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- [`go/adapters/storage/storage_test.go`](../../go/adapters/storage/storage_test.go)
- [`go/adapters/storage/postgres_test.go`](../../go/adapters/storage/postgres_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)

## Related

- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [PostgreSQL should implement the existing storage contract without changing v1](034-postgres-storage-adapter.md)
- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
