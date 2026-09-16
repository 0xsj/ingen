# Custom storage adapters should have reusable contract-test helpers

Applications that provide their own storage backend should be able to run
Amber's semantic checks without copying private repository test code.

## Origin

Amber had shared Go and TypeScript storage suites, but those suites were
internal to the repository. The adapter-authoring guide could describe the
required cases, while external consumers still had to recreate the assertions
for their own implementations.

## What

The repository now provides reusable test helpers:

- Go: [`adapters/storage/contracttest`](../../go/adapters/storage/contracttest/)
  with `RunStoreContract`; and
- TypeScript: the `@0xsj/amber/testing` subpath with
  `runProvenanceStoreContract`.

The helpers check invalid values, first writes, idempotent duplicates,
immutable conflicts, missing records, all three history queries, and
deterministic ordering. They are opt-in test utilities; the main TypeScript
runtime entry point and Amber v1 contracts are unchanged.

## Why

A reusable contract helper turns the generic seam into an enforceable boundary.
Users can replace the example map with a database or service backend and retain
confidence that the storage semantics match Amber's portable behavior.

## Example

Go tests can run the helper against an application-owned store:

```go
contracttest.RunStoreContract(t, func(t testing.TB) amberstorage.Store {
		return newApplicationStore(t)
})
```

TypeScript tests can use the dedicated testing subpath:

```ts
import { runProvenanceStoreContract } from "@0xsj/amber/testing";

await runProvenanceStoreContract(new ApplicationStore());
```

## Gotchas

- The helper verifies Amber semantics; it does not prove a backend's durability,
  transaction, recovery, or production configuration.
- A custom Go store must still honor context cancellation, and a custom backend
  must make insert-if-absent atomic.
- The TypeScript helper is intentionally a separate package subpath so test
  utilities do not expand the main runtime entry point.
- The helper does not cover deployment-owned migrations or broker behavior.

## Used in

- [`go/adapters/storage/contracttest`](../../go/adapters/storage/contracttest/)
- [`typescript/src/testing.ts`](../../typescript/src/testing.ts)
- [`typescript/src/testing.test.ts`](../../typescript/src/testing.test.ts)
- [`docs/adapter-authoring.md`](../adapter-authoring.md)
- [`go/README.md`](../../go/README.md)
- [`typescript/README.md`](../../typescript/README.md)
- [`Makefile`](../../Makefile)

## Related

- [User-defined adapters should reuse Amber's invariant-owning seams](065-user-defined-adapter-authoring.md)
- [Custom backend examples should make the generic seam copyable](066-custom-backend-examples.md)
- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
- [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
