# Custom backend examples should make the generic seam copyable

The generic storage seam is easier to adopt when users can see a minimal
application-owned backend connected to Amber and tested against the important
invariants.

## Origin

Amber documented `KeyValueBackend` for Go and TypeScript, but the built-in
`MapKeyValueBackend` is primarily a reference implementation. Users still had
to translate the interface and contract guidance into an application-shaped
backend before trying the integration.

## What

The repository now includes runnable custom-backend examples for both SDKs:

- [`go/examples/custom-backend`](../../go/examples/custom-backend/); and
- [`typescript/src/custom-backend-example.ts`](../../typescript/src/custom-backend-example.ts).

Each example provides an application-owned map backend, wraps it with
`KeyValueStore`, inserts child/retry/root values out of order, proves duplicate
idempotency, rejects a conflicting value, and checks deterministic history.
Focused tests also verify atomic insert-if-absent behavior and Go buffer
ownership. Both examples run through `make examples`.

## Why

The examples demonstrate the intended division of responsibility: Amber owns
provenance serialization, validation, immutable conflict handling, and history
queries; the application owns the backend's storage mechanism and concurrency
implementation. This improves adoption without adding a database-specific
package or changing the v1 protocol.

## Example

The Go backend can be wrapped directly:

```go
backend := newApplicationKeyValueBackend()
store, err := amberstorage.NewKeyValueStore(backend, "orders/provenance")
```

The TypeScript example uses the same shape:

```ts
const store = new KeyValueStore(
  new ApplicationKeyValueBackend(),
  "orders/provenance",
);
```

## Gotchas

- The map implementations are examples, not durable stores.
- `PutIfAbsent` must be atomic in the real backend, not just in a caller-side
  read-then-write sequence.
- Listing must include every key under the requested namespace prefix.
- The examples do not establish transaction, retention, recovery, or delivery
  guarantees for an application backend.

## Used in

- [`go/examples/custom-backend`](../../go/examples/custom-backend/)
- [`typescript/src/custom-backend.ts`](../../typescript/src/custom-backend.ts)
- [`typescript/src/custom-backend-example.ts`](../../typescript/src/custom-backend-example.ts)
- [`typescript/src/custom-backend.test.ts`](../../typescript/src/custom-backend.test.ts)
- [`Makefile`](../../Makefile)
- [`examples/README.md`](../../examples/README.md)
- [`docs/adapter-authoring.md`](../adapter-authoring.md)

## Related

- [User-defined adapters should reuse Amber's invariant-owning seams](065-user-defined-adapter-authoring.md)
- [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)
- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
- [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
