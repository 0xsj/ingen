# Correlation queries should link related work without changing work membership

Correlation is the cross-work grouping key; it should not be inferred from
either work membership or immediate causation.

## Origin

Amber's model carries `work_id` for one logical work and `causation` for a
direct relationship. Incoming children can create a new work while preserving
the parent correlation ID, leaving storage without a query for the broader
related execution set.

## What

Go stores now expose `ListByCorrelationID(ctx, correlationID)` and TypeScript
stores expose `listByCorrelationId(correlationId)`. They return every stored
execution sharing the requested correlation ID in deterministic depth, attempt,
and execution ID order.

The shared storage fixture verifies three distinct views: work history returns
the root and retry, causation returns the retry and incoming child caused by the
root execution, and correlation history returns all three across both works.

## Why

Keeping the three queries separate preserves the meaning of each identifier:
`work_id` answers “which logical work?”, causation answers “what directly led to
this?”, and `correlation_id` answers “which related execution group?”. This is
important for cross-service inspection and avoids forcing applications to scan
all records themselves.

## Example

```go
related, err := store.ListByCorrelationID(ctx, correlationID)
```

```ts
const related = await store.listByCorrelationId(correlationId);
```

## Gotchas

- Correlation lookup can include executions from multiple work IDs.
- The result is a filtered, deterministic scan, not a causal topological walk.
- TypeScript key-value backends must support namespace listing for this query.
- Empty correlation results are successful and return an empty collection.

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
- [Storage should expose immediate-cause lookup separately from work history](019-causation-query.md)
- [A provenance value is a history node, not a mutable request bag](002-immutable-transitions.md)
