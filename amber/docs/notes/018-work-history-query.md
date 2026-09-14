# Storage needs a deterministic work-history query over execution records

Execution-keyed storage is useful for direct lookup, but logical work is
understood by inspecting its related executions together.

## Origin

The storage contract supported `Put` and `Get` by `execution_id`, while Amber's
model separately exposes `work_id`, depth, and attempt. Applications had no
portable way to retrieve the stored history for one logical work.

## What

Go stores now expose `ListByWorkID`; TypeScript stores expose `listByWorkId`.
Both return all matching executions in deterministic ascending `depth`, then
`attempt`, then `execution_id` order. The Go `FileStore` scans its persisted
snapshot. The TypeScript `KeyValueBackend` adds namespace `list` support so
`KeyValueStore` can enumerate its records without assuming a particular
database.

The shared storage fixture includes a root execution, retry, and incoming child
to verify work membership and ordering in both SDKs. The child has a distinct
`work_id`, so its relationship is checked through the separate causation query.

## Why

Stable ordering makes history inspection reproducible across map-backed,
file-backed, and service-backed implementations. The query stays deliberately
small: it provides work-level retrieval without prematurely claiming to be a
causal graph index or a query language.

## Example

```go
history, err := store.ListByWorkID(ctx, workID)
```

```ts
const history = await store.listByWorkId(workId);
```

## Gotchas

- The result is ordered for deterministic inspection, not guaranteed to be a
  complete causal topological sort when branches share the same depth.
- An empty result is successful and distinct from a missing individual
  execution lookup.
- TypeScript key-value backends must implement namespace listing; a backend
  that only supports direct keys cannot satisfy `KeyValueStore`.
- The query scans the current store. Dedicated indexes and pagination are
  future storage-specific capabilities.

## Used in

- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`go/adapters/storage/storage_test.go`](../../go/adapters/storage/storage_test.go)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`typescript/src/storage.test.ts`](../../typescript/src/storage.test.ts)
- [`typescript/test/conformance.test.mjs`](../../typescript/test/conformance.test.mjs)

## Related

- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
