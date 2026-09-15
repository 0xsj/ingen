# The PostgreSQL schema readiness check should be runnable without writes

Operators need a low-risk way to verify that migrations match the adapter
before enabling provenance writes.

## Origin

Amber exposed `PostgresStore.CheckSchema(ctx)` as a read-only API, but users
still had to write a small program to connect with the configured driver and
invoke it. The existing live integration test intentionally creates schema and
writes test data, so it is not a readiness probe.

## What

The Go examples now include a PostgreSQL schema-check program using the pgx
`database/sql` driver. With `AMBER_POSTGRES_DSN` set, the
`make postgres-schema-check` target:

1. opens and pings the configured database;
2. checks the Amber schema metadata through `CheckSchema`; and
3. prints the expected schema name and version.

It does not call `EnsureSchema`, create tables, or write provenance records.

## Why

Separating readiness verification from bootstrap and integration testing makes
deployment behavior easier to reason about. A mismatch or missing metadata
returns the typed `ErrUnsupportedSchemaVersion` path, while connection and
query failures remain ordinary operational errors.

## Usage

```sh
AMBER_POSTGRES_DSN='postgres://user:password@localhost:5432/amber?sslmode=disable' \
  make postgres-schema-check
```

## Gotchas

- The command requires a reachable database and the application-selected pgx
  driver; it does not perform migrations.
- Use `make postgres-integration` separately for the transaction-isolated full
  behavior check.
- The normal `make release-check` remains offline and does not run either
  PostgreSQL command.

## Used in

- [`go/examples/postgres/main.go`](../../go/examples/postgres/main.go)
- [`Makefile`](../../Makefile)
- [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`docs/notes/040-storage-schema-version-boundary.md`](040-storage-schema-version-boundary.md)

## Related

- [Storage schema versions should be separate from the Amber wire version](040-storage-schema-version-boundary.md)
- [PostgreSQL compatibility should be testable against a real database without entering the default gate](035-postgres-live-integration.md)
- [The optional PostgreSQL import path should have an explicit compatibility check](043-postgres-public-api-boundary.md)
