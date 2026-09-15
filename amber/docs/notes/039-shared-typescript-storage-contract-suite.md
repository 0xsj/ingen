# TypeScript storage backends should prove the same semantic contract

The TypeScript storage boundary should be just as portable and explicit as the
Go boundary.

## Origin

Amber's TypeScript SDK had focused storage tests and shared fixture coverage,
but no reusable suite that every `ProvenanceStore` implementation could run.
That left future browser, filesystem, and service-backed stores without one
clear acceptance bar.

## What

The TypeScript package now includes a shared executable contract suite for
`MemoryStore` and `KeyValueStore`. It verifies:

- non-`Provenance` input rejection;
- idempotent writes and immutable conflict protection;
- direct reads and empty results for missing history;
- work, causation, and correlation queries; and
- deterministic `depth`, `attempt`, and `execution_id` ordering.

The suite inserts a child before its parent and retry, making insertion order
different from the required history order. It runs as part of `npm test` after
the package is compiled. The same executable also runs concurrent duplicate
writes and concurrent history reads against both built-in stores.

## Why

The generic `KeyValueBackend` is intended for user-provided persistence. A
backend should be able to adopt the TypeScript `ProvenanceStore` contract with
confidence that the SDK's append-only and history semantics remain intact.

## Gotchas

- TypeScript has no `context.Context`, so cancellation is not part of this
  suite; asynchronous backend errors remain the backend's responsibility.
- The suite is an executable test artifact, not a runtime export or production
  dependency.
- Backend durability, atomicity, and transaction guarantees still belong to the
  supplied `KeyValueBackend` implementation.

## Used in

- [`typescript/src/storage.contract.test.ts`](../../typescript/src/storage.contract.test.ts)
- [`typescript/src/storage.ts`](../../typescript/src/storage.ts)
- [`typescript/package.json`](../../typescript/package.json)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)

## Related

- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)
- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
