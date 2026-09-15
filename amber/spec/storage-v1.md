# Amber storage contract v1

This document defines the optional language-neutral storage contract for Amber
provenance values. A storage adapter that claims conformance to this contract
MUST preserve these semantics; it MAY expose additional implementation-specific
operations and capabilities.

## 1. Scope

Storage is outside the core provenance value and transport contracts. An
application MAY store Amber values, but storage is not required to create,
propagate, authenticate, or authorize them.

The contract is execution-keyed. It does not claim to be a complete ancestry
graph, a tracing backend, or a general-purpose query language.

## 2. Immutable record semantics

A store maps a validated `Provenance` value by its `execution_id`.

For `Put(value)`:

1. The store MUST validate `value` before writing it.
2. If `execution_id` is new, the store MUST persist the value.
3. If the same `execution_id` already contains the same canonical value, the
   operation MUST succeed idempotently.
4. If the same `execution_id` already contains a different value, the store
   MUST return a conflict and MUST NOT overwrite the existing value.

Equality is evaluated on the canonical serialized Amber value used by the
implementation. Unknown fields MAY be discarded when the implementation does
not advertise lossless decode/encode, consistently with the core wire
contract.

## 3. Direct lookup

`Get(execution_id)` MUST return the stored value when present. A missing
execution MUST be distinguishable from a successful lookup; a language binding
MAY represent this as an absent value or a typed not-found error.

## 4. History queries

All history queries MUST return a successful empty collection when there are no
matches. Results MUST be ordered by ascending:

1. `depth`;
2. `attempt`; and
3. `execution_id`.

### 4.1 Work history

`ListByWorkID(work_id)` MUST return every stored execution whose `work_id`
equals the requested ID, including retries and replays in that logical work.

### 4.2 Immediate causation

`ListByCausation(kind, id)` MUST return every stored execution whose
`causation.kind` and `causation.id` both equal the requested pair. It MUST NOT
recursively walk ancestry or descendants.

### 4.3 Correlation

`ListByCorrelationID(correlation_id)` MUST return every stored execution whose
`correlation_id` equals the requested ID, including executions from distinct
`work_id` values.

## 5. Backend seam

A store MAY be implemented over a caller-supplied key-value backend. Such a
backend MUST provide the equivalent of:

- get one key;
- an atomic insert-if-absent operation; and
- enumeration of keys under a store-owned namespace.

The insert-if-absent operation MUST be atomic for a key under concurrent
callers. A read-then-write sequence is not sufficient. The store layer MUST
retain validation, canonical serialization, conflict handling, and history
filtering rather than delegating those invariants to a generic backend.

The backend owns its durability, transaction, replication, and connection
policy. A backend that cannot enumerate its namespace cannot implement the
history queries through this seam without an additional index or query
capability.

## 6. Persistence schema and migrations

The Amber wire `version` inside each provenance value identifies the value
contract. A persistence container or database schema MAY have its own version,
which MUST be treated as a separate compatibility boundary.

An adapter MUST NOT silently interpret an unsupported persistence schema as the
current schema. It SHOULD expose a typed unsupported-schema result and SHOULD
leave migration ownership with the application or its migration tooling.

Storage-specific schema, indexes, transactions, pagination, retention, and
recovery guarantees are not portable requirements of this contract.

## 7. Conformance

A conforming implementation SHOULD run the shared storage fixture at
[`conformance/storage-v1.json`](../conformance/storage-v1.json) and its
language-level storage contract suite. The fixture covers idempotency,
conflict protection, missing lookup, work history, immediate causation, and
correlation across related work.

The Go and TypeScript SDKs MAY use different asynchronous or error-reporting
shapes, but they MUST preserve the semantic results defined here.
