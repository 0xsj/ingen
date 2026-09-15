# The storage contract should be normative but separate from the core wire contract

Amber's storage semantics need a stable review target without making
persistence a mandatory part of the provenance value or transport protocol.

## Origin

The SDKs had converged on immutable execution-keyed storage, deterministic work,
causation, and correlation queries, generic backend seams, and explicit schema
version boundaries. Those rules lived in implementation docs and tests rather
than in one language-neutral contract.

## What

The new [`spec/storage-v1.md`](../../spec/storage-v1.md) defines the optional
storage contract. It specifies:

- validation, idempotency, conflict protection, and not-found behavior;
- deterministic ordering and exact semantics for all three history queries;
- the atomic insert-if-absent requirement for generic key-value backends;
- the division of responsibility between the store and its backend; and
- the separation between Amber wire versions and persistence schema versions.

The existing shared storage fixture and Go/TypeScript contract suites remain
the executable evidence for the document.

## Why

Separating this document from [`spec/v1.md`](../../spec/v1.md) keeps storage
optional while making adapter behavior reviewable and portable. A future
backend can implement the contract without adopting PostgreSQL's schema or
either SDK's internal error type.

## Example

An adapter review can start with the language-neutral contract and then run the
shared suites:

```sh
sed -n '1,220p' spec/storage-v1.md
make check
```

The contract remains optional for applications that do not need provenance
persistence.

## Gotchas

- The storage contract does not promise durability, transactions, pagination,
  retention, recovery, or a complete ancestry graph.
- Language bindings may represent missing values and cancellation differently
  as long as semantic behavior is preserved.
- Persistence migrations remain application-owned; the adapter must detect an
  unsupported schema rather than silently treating it as current.

## Used in

- [`spec/storage-v1.md`](../../spec/storage-v1.md)
- [`conformance/storage-v1.json`](../../conformance/storage-v1.json)
- [`go/adapters/storage/storage_contract_test.go`](../../go/adapters/storage/storage_contract_test.go)
- [`typescript/src/storage.contract.test.ts`](../../typescript/src/storage.contract.test.ts)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)

## Related

- [Every Go storage adapter should prove the same semantic contract](038-shared-go-storage-contract-suite.md)
- [TypeScript storage backends should prove the same semantic contract](039-shared-typescript-storage-contract-suite.md)
- [Storage schema versions should be separate from the Amber wire version](040-storage-schema-version-boundary.md)
