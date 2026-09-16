# Authoring Amber adapters

Amber keeps the provenance model independent from frameworks, brokers, and
databases. An adapter connects those application-owned systems to Amber while
preserving the v1 contracts.

This guide is for teams that need a backend or boundary not included in the
repository. The optional PostgreSQL package is a concrete convenience; the
generic seams below are the extension points for user-defined implementations.

## Choose the smallest seam

Use the narrowest interface that matches the system you are integrating:

| Need | Go | TypeScript |
| --- | --- | --- |
| Persist complete provenance values | `amberstorage.Store` | `ProvenanceStore` |
| Reuse Amber serialization and history queries | `amberstorage.KeyValueBackend` | `KeyValueBackend` |
| Carry provenance across a boundary | transport/header or envelope adapter | transport/header or envelope adapter |
| Add observability fields | logging/tracing projection | logging/tracing projection |

For a new durable store, prefer the key-value seam when the backend can provide
an atomic insert-if-absent operation and namespace listing. Amber's
`KeyValueStore` then owns validation, JSON serialization, immutable conflict
handling, and deterministic history queries. Implement `Store` directly only
when the backend's native query or transaction model makes that clearer.

## Storage invariants

Every storage implementation must preserve these behaviors:

1. A valid provenance value is stored by `execution_id`.
2. Inserting the exact same serialized value again is idempotent.
3. Inserting a different value with an existing `execution_id` returns a
   conflict and never overwrites the original.
4. Missing records are distinguishable from backend failures.
5. Work, causation, and correlation queries return all matching values in
   deterministic order: ascending `depth`, then `attempt`, then
   `execution_id`.
6. Invalid provenance values are rejected before persistence.
7. A backend must not expose mutable storage-owned buffers to callers.

The Go key-value interface is:

```go
type KeyValueBackend interface {
	Get(context.Context, string) ([]byte, error)
	PutIfAbsent(context.Context, string, []byte) (bool, error)
	List(context.Context, string) ([]string, error)
}
```

`PutIfAbsent` must be atomic for a key under concurrent callers. `Get` returns
`nil, nil` for a missing key. `List` must return every key under the requested
prefix; `KeyValueStore` performs the provenance filtering and ordering.

The TypeScript interface has the same shape with asynchronous string values:

```ts
interface KeyValueBackend {
  get(key: string): Promise<string | undefined>;
  putIfAbsent(key: string, value: string): Promise<boolean>;
  list(prefix: string): Promise<readonly string[]>;
}
```

`putIfAbsent` returns `true` only when it inserted the key. It must remain
atomic when multiple promises attempt the same key. `undefined` represents a
missing key; backend failures must reject rather than look like missing data.

## Namespace and ownership rules

Construct a `KeyValueStore` with an application-specific namespace when one
backend serves more than one logical store:

```go
store, err := amberstorage.NewKeyValueStore(backend, "orders/provenance")
```

```ts
const store = new KeyValueStore(backend, "orders/provenance");
```

The wrapper owns the namespace prefix and Amber serialization. The backend
owns connection management, durability, transactions, retries, retention,
replication, and recovery policy. Do not use a non-atomic read-then-write
sequence to implement `PutIfAbsent`.

## Boundary adapters

For HTTP or messaging, decode the existing Amber transport value, apply the
application's incoming policy and optional trust validator, then install the
result only within the request or message scope. Derive a child execution for
work performed by the consumer and explicitly propagate that child on the
outgoing boundary.

For logging, tracing, and OpenTelemetry, project Amber's stable `amber.*`
fields onto application-owned records or spans. The adapter should not create
span lifecycle semantics, decide authorization, or claim that provenance proves
the operation was correct.

## Contract-test checklist

Before treating a custom adapter as compatible, test at least:

- first insert and read-back;
- duplicate insert of the same value;
- conflicting insert with the same execution ID;
- missing lookup and backend error handling;
- work, causation, and correlation queries;
- deterministic ordering independent of insertion order;
- invalid-value rejection;
- concurrent duplicate writes; and
- context cancellation for Go methods.

The repository's Go suite is in
[`go/adapters/storage/storage_contract_test.go`](../go/adapters/storage/storage_contract_test.go),
and the TypeScript suite is in
[`typescript/src/storage.contract.test.ts`](../typescript/src/storage.contract.test.ts).
Use those cases as the acceptance checklist for a custom implementation; the
tests intentionally cover semantics rather than a particular database.
The copyable examples are
[`go/examples/custom-backend`](../go/examples/custom-backend/) and
[`typescript/src/custom-backend-example.ts`](../typescript/src/custom-backend-example.ts).
For reusable checks, Go users can import
[`adapters/storage/contracttest`](../go/adapters/storage/contracttest/) in
tests, and TypeScript users can import `runProvenanceStoreContract` from the
`@0xsj/amber/testing` subpath.

## What remains application-owned

An adapter does not make these decisions for the application:

- authentication, authorization, or trust establishment;
- transaction boundaries and delivery semantics;
- connection pooling and shutdown;
- migration history and deployment sequencing;
- retention, archival, backups, and recovery; or
- broker-, framework-, or database-specific configuration.

Keep those policies in the deployment or integration layer. Extend the Amber
specification only when a consumer requires a new cross-language contract,
not merely because one backend exposes a new feature.

## Related contracts

- [`spec/v1.md`](../spec/v1.md) — core provenance and boundary behavior
- [`spec/storage-v1.md`](../spec/storage-v1.md) — language-neutral storage semantics
- [`spec/trust-v1.md`](../spec/trust-v1.md) — structural and application trust boundaries
- [`adapters/storage/README.md`](../adapters/storage/README.md) — built-in storage implementations
- [`docs/getting-started.md`](getting-started.md) — smallest adoption path
