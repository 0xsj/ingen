# Every Go storage adapter should prove the same semantic contract

Storage implementations should be interchangeable at the `Store` boundary,
so their core behavior needs one reusable test suite.

## Origin

Amber had focused tests for the memory, file, generic key-value, and PostgreSQL
stores, but those tests did not apply the same assertions to every
implementation. PostgreSQL history queries also used insertion sequence even
though the contract requires deterministic Amber history ordering.

## What

The Go storage package now provides an internal `runStoreContract` test harness.
It exercises every local `Store` implementation for:

- validation, idempotent writes, and conflict protection;
- direct reads and missing execution IDs;
- work, causation, and correlation queries;
- cancellation behavior; and
- deterministic `depth`, `attempt`, and `execution_id` ordering.

The harness inserts a child before its parent and retry, making insertion order
different from the required history order. The opt-in live PostgreSQL test uses
the same suite after the application-managed migration has been applied and
the schema has been verified. PostgreSQL queries now order directly from the
stored JSONB `depth` and `attempt` fields, followed by `execution_id`.

## Why

A backend that satisfies only `Put` and `Get` can still return misleading
history or silently diverge on cancellation and conflict semantics. One suite
keeps the portable contract explicit and gives future adapters a concrete
acceptance bar.

## Example

Run the offline implementations through the shared contract with:

```sh
cd go
go test ./adapters/storage -run 'Test.*StoreContract' -count=1
```

Run the same contract against a migrated PostgreSQL database with
`AMBER_POSTGRES_DSN=... make postgres-integration`.

## Gotchas

- The suite is an internal Go test helper, not a runtime dependency or public
  testing API.
- A live PostgreSQL server is still required to execute the PostgreSQL branch;
  the default release gate remains offline.
- Storage-specific durability, transactions, migrations, and indexes remain
  outside the portable `Store` contract.

## Used in

- [`go/adapters/storage/storage_contract_test.go`](../../go/adapters/storage/storage_contract_test.go)
- [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- [`go/adapters/storage/postgres_integration_test.go`](../../go/adapters/storage/postgres_integration_test.go)
- [`adapters/storage/README.md`](../../adapters/storage/README.md)

## Related

- [Storage must distinguish idempotent re-insertion from overwriting history](010-append-only-storage-contract.md)
- [Storage needs a deterministic work-history query over execution records](018-work-history-query.md)
- [A generic storage backend should own persistence while Amber owns invariants](036-generic-storage-backend-seam.md)
