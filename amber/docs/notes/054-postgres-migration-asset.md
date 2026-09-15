# PostgreSQL should ship a checked-in migration asset with readiness guidance

Application-owned migrations are easier to review and deploy when the adapter
provides a versioned SQL artifact that is kept aligned with its runtime schema.

## Origin

Amber exposed `PostgresSchema` as a Go string and correctly left migration
ownership with the application. A deployment using a migration tool still had
to copy that SQL into its own migration, which created an avoidable drift risk.

## What

The optional `amberpostgres` package now embeds
`migrations/001_amber_provenance.sql` as `SchemaMigrationV1`. A public-package
test compares the trimmed asset with `PostgresSchema`, so changes to one
representation require the other to be updated.

The deployment sequence is documented as:

1. apply the v1 SQL asset through application-owned migration tooling;
2. construct the store with the application's driver and pool;
3. run `CheckSchema` as a read-only readiness check; and
4. enable provenance writes only after readiness succeeds.

`EnsureSchema` remains available as an idempotent bootstrap path for simple
applications, but no migration or connection lifecycle becomes mandatory.

## Why

This improves reviewability and operational handoff without turning Amber into
a migration engine. The SQL asset, schema metadata version, and read-only
probe now form one explicit deployment boundary.

## Used in

- [`go/adapters/storage/postgres/migrations/001_amber_provenance.sql`](../../go/adapters/storage/postgres/migrations/001_amber_provenance.sql)
- [`go/adapters/storage/postgres/postgres.go`](../../go/adapters/storage/postgres/postgres.go)
- [`go/adapters/storage/postgres/postgres_test.go`](../../go/adapters/storage/postgres/postgres_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [Storage schema versions should be separate from the Amber wire version](040-storage-schema-version-boundary.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
- [CI should verify PostgreSQL read-only schema readiness](048-postgres-ci-schema-readiness-check.md)
