# PostgreSQL compatibility should be testable against a real database without entering the default gate

SQL mocks verify query construction and invariant handling, but only a real
PostgreSQL server can validate the driver, SQL dialect, JSONB behavior, schema,
indexes, and transaction interaction together.

## Origin

Amber's PostgreSQL adapter initially had offline `go-sqlmock` coverage so the
release gate stayed deterministic. A deployment still needed an explicit path
to validate the adapter against its PostgreSQL version and driver.

## What

The storage package now has an opt-in `TestPostgresStoreLive` test. When
`AMBER_POSTGRES_DSN` is set, it opens PostgreSQL through the official pgx
`database/sql` compatibility driver, pings the database, begins a transaction,
checks that the application-managed schema is ready, exercises writes,
idempotency, reads, work/causation/correlation queries, and conflict detection,
then rolls the transaction back. The `make postgres-integration` target
requires the DSN and an already-migrated database before running it.

## Why

This separates two useful guarantees: the normal release gate is safe to run
offline, while a deployment owner has a repeatable live compatibility check.
Rolling back the transaction avoids leaving test provenance records behind and
also proves the store works with an application-managed `*sql.Tx`.

## Example

```sh
AMBER_POSTGRES_DSN='postgres://user:password@localhost:5432/amber?sslmode=disable' \
  make postgres-integration
```

## Gotchas

- The live test requires a reachable PostgreSQL database with the Amber
  migration already applied and appropriate read/write privileges.
- Use a disposable or isolated database. The test runs all provenance writes
  inside a transaction and rolls them back when finished; it does not apply
  migrations or create tables.
- `make release-check` intentionally does not run this target because it must
  not make network connections or depend on secrets.
- The Go application still chooses the driver, pool, migration lifecycle, and
  production transaction boundaries.

## Used in

- [`go/adapters/storage/postgres_integration_test.go`](../../go/adapters/storage/postgres_integration_test.go)
- [`Makefile`](../../Makefile)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`docs/notes/034-postgres-storage-adapter.md`](034-postgres-storage-adapter.md)

## Related

- [PostgreSQL should implement the existing storage contract without changing v1](034-postgres-storage-adapter.md)
- [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
