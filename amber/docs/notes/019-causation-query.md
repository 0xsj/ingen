# Storage should expose immediate-cause lookup separately from work history

Work history and causal relationships answer different questions and should
not be conflated by storage APIs.

## Origin

Amber's `Child`, `Retry`, and `Replay` transitions create new execution and
causation relationships. A child normally receives a new `work_id`, so a
`ListByWorkID` query cannot find it from the parent's work history.

## What

Go stores now expose `ListByCausation(ctx, kind, id)` and TypeScript stores
expose `listByCausation(kind, id)`. They return records whose immediate
causation kind and ID match the requested pair, using the same deterministic
depth, attempt, and execution ID ordering as work-history queries.

The shared storage fixture now models a root execution, a retry in the same
work, and an incoming child in a distinct work. It verifies that work lookup
returns only the root and retry, while causation lookup returns the retry and
child caused by the root execution.

## Why

Separating work membership from causation preserves the meaning of
`work_id` and makes cross-work relationships inspectable. It also leaves room
for future recursive lineage or graph indexes without changing the basic store
contract.

## Example

```go
children, err := store.ListByCausation(ctx, "execution", executionID)
```

```ts
const children = await store.listByCausation("execution", executionId);
```

## Gotchas

- The query returns immediate causes only; it does not recursively walk an
  ancestry or descendant graph.
- A retry and a child can share the same causation execution while belonging to
  different logical works.
- Results are deterministically ordered for inspection, not guaranteed to be a
  complete topological traversal.
- The storage backends scan their records. Production deployments may add
  indexes behind the same interface.

## Used in

- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`go/adapters/storage/storage_test.go`](../../go/adapters/storage/storage_test.go)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`typescript/src/storage.test.ts`](../../typescript/src/storage.test.ts)
- [`typescript/test/conformance.test.mjs`](../../typescript/test/conformance.test.mjs)

## Related

- [Storage needs a deterministic work-history query over execution records](018-work-history-query.md)
- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [Correlation queries should link related work without changing work membership](020-correlation-query.md)
