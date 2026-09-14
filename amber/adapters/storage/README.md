# Storage adapter

The Amber storage adapter defines a small append-only store contract for
provenance values and provides an in-memory reference implementation for tests
and examples.

Records are keyed by `execution_id`:

- inserting a new execution stores it;
- inserting the exact same value again is idempotent; and
- inserting a different value with an existing execution ID returns a conflict
  instead of overwriting history.

The store validates values before writing and returns copies/read-only values on
read. `MemoryStore` is process-local and makes no durability, transaction, or
crash-recovery guarantee. Database adapters belong behind the same contract.

## Implementations

- Go: [`go/adapters/storage`](../../go/adapters/storage/)
- TypeScript: [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- Shared storage vector: [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
