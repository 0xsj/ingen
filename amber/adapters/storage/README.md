# Storage adapter

The Amber storage adapter defines a small append-only store contract for
provenance values and provides an in-memory reference implementation for tests
and examples.

Records are keyed by `execution_id`:

- inserting a new execution stores it;
- inserting the exact same value again is idempotent; and
- inserting a different value with an existing execution ID returns a conflict
  instead of overwriting history.

Stores also expose `ListByWorkID` in Go and `listByWorkId` in TypeScript. These
queries return every stored execution for a logical work, ordered by ascending
`depth`, then `attempt`, then `execution_id`. No matching records returns an
empty collection.

Stores also expose `ListByCausation` in Go and `listByCausation` in TypeScript.
These queries return executions whose immediate causation kind and ID match the
requested pair, using the same deterministic ordering. This is the portable
way to find a child work or a retry caused by a specific execution.

Stores also expose `ListByCorrelationID` in Go and `listByCorrelationId` in
TypeScript. These queries return all stored executions sharing a correlation
ID, including related executions from distinct logical works.

The store validates values before writing and returns copies/read-only values on
read. `MemoryStore` is process-local and makes no durability, transaction, or
crash-recovery guarantee. Database adapters belong behind the same contract.

## Durable backend seam

The Go adapter also provides `FileStore`, which writes an atomically replaced
JSON snapshot and can be reopened by another `FileStore` instance. It is safe
for concurrent use within one process, but it does not provide cross-process
locking or database transactions; the parent directory must already exist.

The Go adapter also provides `KeyValueBackend` and `KeyValueStore`. A runtime
can supply any backend that supports `Get`, atomic `PutIfAbsent`, and prefix
listing; Amber keeps validation, JSON serialization, append-only conflict
handling, and history queries above that seam. `MapKeyValueBackend` is the
concurrency-safe process-local reference backend:

```go
backend := amberstorage.NewMapKeyValueBackend()
store, err := amberstorage.NewKeyValueStore(backend, "service/provenance")
if err != nil {
    return err
}
return store.Put(ctx, provenance)
```

The TypeScript adapter provides `KeyValueBackend` and `KeyValueStore`. A runtime
can supply an IndexedDB, filesystem, or service-backed implementation as long
as `putIfAbsent` is atomic and `list` can enumerate a namespace prefix.
`MapKeyValueBackend` is a small process-local
reference backend for tests and examples.

## PostgreSQL

The Go adapter also provides `PostgresStore`, which uses the standard
`database/sql` surface and PostgreSQL's `JSONB` storage plus indexed query
columns. Call `EnsureSchema` during application setup, or apply the exported
`PostgresSchema` through the application's migration system:

```go
db, err := sql.Open("your-postgres-driver", dsn)
if err != nil {
    return err
}
store, err := amberstorage.NewPostgresStore(db)
if err != nil {
    return err
}
if err := store.EnsureSchema(ctx); err != nil {
    return err
}
return store.Put(ctx, provenance)
```

The store uses `ON CONFLICT (execution_id) DO NOTHING` and compares an
existing value before returning success, preserving idempotent writes and
conflict rejection. The application supplies the PostgreSQL driver and owns
connection pooling, migrations, transactions, and shutdown.

For live verification against a disposable or transaction-isolated database,
set `AMBER_POSTGRES_DSN` and run:

```sh
make postgres-integration
```

The live test runs all storage operations inside a transaction and rolls it
back when finished. The standard `make release-check` remains offline.

## Implementations

- Go: [`go/adapters/storage`](../../go/adapters/storage/)
- Go generic key-value seam: [`go/adapters/storage/keyvalue.go`](../../go/adapters/storage/keyvalue.go)
- Go PostgreSQL: [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- TypeScript: [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- Shared storage vector: [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
