# Storage must distinguish idempotent re-insertion from overwriting history

An execution record can be written repeatedly without creating a conflict only
when the repeated value is identical; a different value for the same execution
ID must be rejected.

## Origin

The storage slice needed a useful persistence boundary before choosing a database
or adding durability-specific dependencies.

## What

The storage contract keys immutable provenance by `execution_id`. `Put` validates
the value, stores a new record, treats the exact same serialized value as an
idempotent write, and returns a conflict for a different value with the same
key. `Get` returns the stored value or a not-found result. `ListByWorkID` in Go
and `listByWorkId` in TypeScript return all stored executions for a logical work
in deterministic `depth`, `attempt`, and `execution_id` order. Go accepts a
context for cancellation; TypeScript exposes the same operations
asynchronously.

`ListByCausation` in Go and `listByCausation` in TypeScript provide the
corresponding immediate-cause lookup by causation kind and ID.

The current `MemoryStore` implementations are concurrency-safe/process-local
references, not durable databases.

## Why

Silently overwriting a record would allow a later retry, adapter bug, or caller
mutation to rewrite the provenance of an execution that has already been
observed. Treating identical writes as idempotent supports at-least-once
delivery and safe retries, while conflict detection preserves the append-only
meaning of an execution ID.

## Example

```text
Put(E1, value A) -> stored
Put(E1, value A) -> idempotent success
Put(E1, value B) -> conflict; value A remains readable
Get(E2)          -> not found
ListByWorkID(W1) -> [E1, E2, ...] in deterministic history order
ListByCausation(execution, E1) -> direct children/retries
```

## Gotchas

- A memory store proves adapter semantics, not crash recovery, transactions,
  replication, or durable ordering.
- The key remains `execution_id`; work-level history is a scan/query over those
  records and does not yet provide a causal graph index.
- Equality uses the canonical serialized provenance value, so wire-shape
  changes are storage-visible changes.
- A delivery retry is not automatically an Amber `Retry` transition; callers
  still choose the provenance operation before persisting it.

## Used in

- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`go/adapters/storage/storage_test.go`](../../go/adapters/storage/storage_test.go)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`typescript/src/storage.test.ts`](../../typescript/src/storage.test.ts)
- [`conformance/storage-v1.json`](../../conformance/storage-v1.json)

## Related

- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
- [Message metadata should reuse the transport envelope without sharing mutable maps](007-messaging-metadata-adapter.md)
- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
