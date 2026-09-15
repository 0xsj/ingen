# Vendor-specific storage should have an explicit optional import path

The generic storage seam should be the default extension point, while a
maintained database integration should be visibly vendor-specific.

## Origin

Amber now has a Go `KeyValueBackend` seam for user-defined persistence and a
PostgreSQL implementation for teams that want a ready-made SQL option. Both
were initially exposed from the same storage package, which made the optional
boundary less obvious to readers of the import graph.

## What

The package `go/adapters/storage/postgres` now exposes the PostgreSQL store
through the explicit `amberpostgres` import path while preserving the existing
implementation and behavior. The external Go consumer smoke test imports that
optional package path. User-defined backends continue to use
`go/adapters/storage` and its `KeyValueBackend`/`KeyValueStore` seam.

## Why

The import path communicates the architectural choice: PostgreSQL is a
convenience integration, not Amber's universal storage implementation. Users
who want a different persistence system can depend only on the generic storage
package and implement the small backend contract.

## Example

```go
import amberpostgres "github.com/0xsj/ingen/amber/adapters/storage/postgres"

store, err := amberpostgres.NewPostgresStore(db)
```

```go
import amberstorage "github.com/0xsj/ingen/amber/adapters/storage"

backend := amberstorage.NewMapKeyValueBackend()
store, err := amberstorage.NewKeyValueStore(backend, "service/provenance")
```

## Gotchas

- The PostgreSQL implementation remains available from its original parent
  package during v1 development; new application code should prefer the
  explicit vendor package path.
- The generic backend seam does not choose database schema, driver, query
  language, or transaction policy.
- PostgreSQL's live integration check remains opt-in and is separate from the
  offline release gate.

## Used in

- [`go/adapters/storage/postgres/postgres.go`](../../go/adapters/storage/postgres/postgres.go)
- [`go/examples/consumer/main.go`](../../go/examples/consumer/main.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`docs/notes/034-postgres-storage-adapter.md`](034-postgres-storage-adapter.md)

## Related

- [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)
- [PostgreSQL should implement the existing storage contract without changing v1](034-postgres-storage-adapter.md)
- [The public API should change only through an explicit compatibility check](032-public-api-compatibility.md)
