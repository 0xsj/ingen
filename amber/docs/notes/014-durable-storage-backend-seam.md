# Durable storage should preserve append-only semantics behind an explicit backend seam

Amber can provide useful durability primitives without selecting one database
or pretending a local file has database-level concurrency guarantees.

## Origin

The storage contract had process-local `MemoryStore` implementations, but its
next users needed a path to persistence while keeping the core SDK independent
of database drivers and runtime-specific storage APIs.

## What

The Go adapter provides `FileStore`, which stores an execution-keyed JSON
snapshot and commits successful writes through a temporary file followed by an
atomic rename. It preserves idempotent re-insertion, rejects changed values
for an existing execution ID, and scans snapshots for deterministic work
history queries.

The TypeScript adapter provides `KeyValueBackend` and `KeyValueStore`. The
backend must offer `get` and an atomic `putIfAbsent`, allowing IndexedDB,
filesystem, or service-backed implementations to satisfy the same append-only
contract. `MapKeyValueBackend` provides a small process-local reference backend
for tests and implements namespace listing for work history queries.

## Why

Durability and storage ownership vary by deployment, but rewriting an observed
execution must remain invalid everywhere. An explicit backend seam lets Amber
preserve that invariant without adding a database dependency to either SDK.

## Example

```go
store, err := amberstorage.NewFileStore("./var/amber-provenance.json")
if err != nil {
    return err
}
return store.Put(ctx, provenance)
```

```ts
const store = new KeyValueStore(indexedDbBackend, "amber/provenance");
await store.put(provenance);
const saved = await store.get(provenance.execution_id);
```

## Gotchas

- `FileStore` is process-safe, not a cross-process transactional database.
- The Go file store expects its parent directory to exist and uses atomic
  snapshot replacement rather than an append-only physical log.
- TypeScript backends must make `putIfAbsent` atomic; a read-then-write wrapper
  would reintroduce a race between conflicting writers.
- The storage key remains `execution_id`; work-history and immediate-causation
  queries scan records, while recursive causal graph indexes remain future
  capabilities.

## Used in

- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`go/adapters/storage/storage_test.go`](../../go/adapters/storage/storage_test.go)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`typescript/src/storage.test.ts`](../../typescript/src/storage.test.ts)
- [`README.md`](../../README.md)

## Related

- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [A project needs one verification command that crosses its language boundaries](011-root-verification-command.md)
- [Messaging middleware should own context and metadata at the consumer boundary](013-messaging-boundary-middleware.md)
