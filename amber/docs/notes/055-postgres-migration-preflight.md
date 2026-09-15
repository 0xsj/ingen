# PostgreSQL migration readiness should have an offline preflight

Deployment pipelines need a fast check for migration drift before they connect
to a database, while runtime readiness must still reject an absent schema.

## Origin

Amber now ships and tests a checked-in PostgreSQL migration asset, and
`CheckSchema` reports unsupported schema metadata. The asset check was only
available indirectly through the full release gate, and the missing-metadata
failure path was not explicitly covered by a focused SQL-mock test.

## What

The project now provides `make postgres-migration-check`, which runs the public
PostgreSQL package contract test offline. That test confirms the embedded
`SchemaMigrationV1` asset remains aligned with `PostgresSchema`.

The adapter test suite also verifies that `CheckSchema` returns
`ErrUnsupportedSchemaVersion` when the schema metadata row is missing. The
offline preflight does not parse or execute SQL and does not replace the live
readiness command.

## Why

This separates three checks with different responsibilities:

- migration asset drift is caught without network or database access;
- a live integration job exercises PostgreSQL behavior; and
- a deployment readiness probe confirms that application-managed migrations
  are present before writes are enabled.

## Example

Run the offline asset check before connecting to a database:

```sh
make postgres-migration-check
```

Then use the live migration, readiness, and integration sequence when
PostgreSQL is in scope.

## Gotchas

- The preflight does not parse or execute SQL and cannot replace a live
  database check.
- `postgres-schema-check` still requires an already-applied migration.

## Used in

- [`Makefile`](../../Makefile)
- [`go/adapters/storage/postgres/postgres_test.go`](../../go/adapters/storage/postgres/postgres_test.go)
- [`go/adapters/storage/postgres_test.go`](../../go/adapters/storage/postgres_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [PostgreSQL should ship a checked-in migration asset with readiness guidance](054-postgres-migration-asset.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
- [CI should verify PostgreSQL read-only schema readiness](048-postgres-ci-schema-readiness-check.md)
