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

The TypeScript adapter provides `KeyValueBackend` and `KeyValueStore`. A runtime
can supply an IndexedDB, filesystem, or service-backed implementation as long
as `putIfAbsent` is atomic and `list` can enumerate a namespace prefix.
`MapKeyValueBackend` is a small process-local
reference backend for tests and examples.

## Implementations

- Go: [`go/adapters/storage`](../../go/adapters/storage/)
- TypeScript: [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- Shared storage vector: [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
