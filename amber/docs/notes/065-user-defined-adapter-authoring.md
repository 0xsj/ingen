# User-defined adapters should reuse Amber's invariant-owning seams

Amber's deployment-specific release work can be backlogged while the library
improves adoption guidance for applications that need their own backend or
boundary adapter.

## Origin

Amber already exposes generic Go and TypeScript storage seams, and the v1
specification keeps integrations outside the core. The implementation notes
explained why those seams exist, but a new adapter author still had to combine
the interface details, invariants, and test expectations from several files.

## What

The repository now has an
[`adapter-authoring.md`](../adapter-authoring.md) guide covering:

- choosing `Store`/`ProvenanceStore` versus the generic key-value seam;
- atomic insert-if-absent and immutable conflict semantics;
- namespace, ownership, and missing-value behavior;
- HTTP, messaging, logging, tracing, and trust-boundary responsibilities; and
- a contract-test checklist for custom implementations.

The guide is linked from the root and SDK onboarding documentation. No runtime
API, wire field, storage contract, or deployment behavior changed.

## Why

The generic seam is most valuable when users can implement their own backend
without accidentally weakening Amber's invariants. Centralizing the authoring
guidance improves portability while preserving the decision that database-,
broker-, and framework-specific behavior belongs to applications and adapters.

## Example

A user-provided key-value backend can be wrapped without reimplementing Amber's
serialization or history queries:

```go
store, err := amberstorage.NewKeyValueStore(backend, "orders/provenance")
```

The backend must make `PutIfAbsent` atomic; the wrapper handles validation,
idempotency, conflicts, and deterministic work/causation/correlation queries.

## Gotchas

- A read-then-write implementation is not a valid atomic `PutIfAbsent`.
- Backend durability, transactions, retries, retention, and recovery remain
  application-owned.
- Custom adapters must preserve deterministic query ordering and never turn
  backend errors into missing records.
- This guidance does not authorize new core fields, transitions, or signed
  transport semantics.

## Used in

- [`docs/adapter-authoring.md`](../adapter-authoring.md)
- [`README.md`](../../README.md)
- [`go/README.md`](../../go/README.md)
- [`typescript/README.md`](../../typescript/README.md)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)
- [`go/adapters/storage/storage_contract_test.go`](../../go/adapters/storage/storage_contract_test.go)
- [`typescript/src/storage.contract.test.ts`](../../typescript/src/storage.contract.test.ts)

## Related

- [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)
- [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
- [Amber needs a release-readiness matrix before more generic expansion](046-release-readiness-matrix.md)
