# PostgreSQL should implement the existing storage contract without changing v1

The first production persistence target should preserve Amber's append-only
semantics and keep database ownership with the application.

## Origin

Amber had memory and file-backed stores, but a service deployment needed a
durable database path. The storage contract already defined the operations and
invariants; the missing piece was a concrete PostgreSQL implementation.

## What

The Go storage adapter now provides `PostgresStore` over the small
`database/sql` executor surface. `PostgresSchema` creates an execution-keyed
table with `JSONB` value storage, a monotonic sequence for deterministic query
order, and indexes for work, causation, and correlation lookups.

`Put` validates before writing, uses PostgreSQL `ON CONFLICT ... DO NOTHING`,
and compares an existing value so identical writes remain idempotent while a
different value for the same execution ID returns `ErrConflict`. Reads decode
the stored v1 JSON through Amber's normal validation path.

## Why

The database adapter can provide real durability and indexed lookups without
adding a driver or connection-pool policy to Amber. Accepting both `*sql.DB`
and `*sql.Tx` lets applications decide how transactions fit their workflow,
while the existing `Store` interface keeps core semantics unchanged.

## Example

```go
store, err := amberstorage.NewPostgresStore(db)
if err != nil {
    return err
}
if err := store.EnsureSchema(ctx); err != nil {
    return err
}
if err := store.Put(ctx, provenance); err != nil {
    return err
}
```

## Gotchas

- The adapter uses PostgreSQL SQL syntax and requires the application to
  provide a compatible `database/sql` driver.
- `EnsureSchema` is a convenient idempotent setup path; teams with migration
  tooling can apply `PostgresSchema` themselves.
- The adapter stores the complete JSON value and duplicates query fields for
  indexes. Future schema migrations must preserve the stored v1 value and
  append-only execution identity.
- Tests use `go-sqlmock`; no live PostgreSQL server is required by the local
  verification gates. A deployment should add a live integration test against
  its managed PostgreSQL version before rollout.
- The opt-in `make postgres-integration` target uses `AMBER_POSTGRES_DSN`, a
  `pgx/v5/stdlib` database/sql driver, and a transaction that is rolled back at
  test completion. It is the live compatibility check and should use a
  disposable or isolated database.

## Used in

- [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- [`go/adapters/storage/postgres_test.go`](../../go/adapters/storage/postgres_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`README.md`](../../README.md)

## Related

- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
