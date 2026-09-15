# A generic storage backend should own persistence while Amber owns invariants

The portable storage abstraction is a contract plus a small backend seam, not
a pretend universal database implementation.

## Origin

Amber's `Store` contract was already portable, but the Go SDK offered only
concrete memory and file stores alongside the optional PostgreSQL adapter.
Users needed a straightforward way to connect their own database, cache,
filesystem, or service without copying Amber's serialization and conflict
logic.

## What

The Go storage adapter now provides `KeyValueBackend` and `KeyValueStore`.
Backends implement `Get`, atomic `PutIfAbsent`, and prefix `List`. The store
layer validates provenance, serializes the complete value, enforces
idempotent-versus-conflicting writes, decodes reads, and performs work,
causation, and correlation queries. `MapKeyValueBackend` is a concurrent
process-local reference backend for tests and examples.

## Why

This keeps the general implementation database-agnostic at the useful seam:
users define how bytes are stored, while Amber defines what a valid immutable
provenance record means. PostgreSQL remains an optional convenience adapter for
teams that want a maintained SQL schema and indexed queries. Other backends do
not need to adopt PostgreSQL's types or SQL dialect.

## Example

```go
backend := amberstorage.NewMapKeyValueBackend()
store, err := amberstorage.NewKeyValueStore(backend, "service/provenance")
if err != nil {
    return err
}
return store.Put(ctx, provenance)
```

An application can replace `MapKeyValueBackend` with a backend backed by its
own database or service while retaining the same `Store` behavior.

## Gotchas

- `PutIfAbsent` must be atomic for a key. A read-then-write implementation is
  not sufficient under concurrent writers.
- `List` returns keys under a namespace prefix; the backend owns enumeration,
  while Amber owns provenance filtering and deterministic history ordering.
- The Go backend uses `nil, nil` from `Get` to represent a missing key and
  returns `ErrNotFound` at the `Store` level.
- The PostgreSQL adapter remains useful as a concrete option, but it is not
  the portable storage contract and does not belong in the core package.

## Used in

- [`go/adapters/storage/keyvalue.go`](../../go/adapters/storage/keyvalue.go)
- [`go/adapters/storage/keyvalue_test.go`](../../go/adapters/storage/keyvalue_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)

## Related

- [PostgreSQL should implement the existing storage contract without changing v1](034-postgres-storage-adapter.md)
- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
