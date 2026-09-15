# The messaging reference vertical should preserve consumer-owned child metadata

Message middleware should make broker integration portable while leaving
consumer execution and outgoing message choices with the application.

## Origin

Amber had broker-neutral messaging adapters and tests, but no reference
consumer flow showing trust validation, child persistence, and reply metadata
together. Both SDKs also rewrote an explicitly supplied child provenance value
with the inbound parent when middleware completed.

## What

The Go and TypeScript messaging middleware now preserve an existing outgoing
provenance metadata value. When the consumer does not select one, middleware
continues to propagate the inbound value as the default.

Both SDKs now include broker-neutral reference consumers that:

- accept an inbound message or create local provenance for a top-level message;
- apply an optional application-owned validator before consumer logic;
- derive and persist an `OriginIncoming` child; and
- return a processed reply carrying the child provenance metadata.

The examples use in-process messages and application-owned memory stores, so
no broker or database is required.

## Why

This keeps the messaging behavior aligned with the HTTP boundary and makes the
consumer-owned transition explicit. A broker-specific adapter can supply
delivery, retry, acknowledgement, and transaction semantics without changing
Amber's portable message contract.

## Used in

- [`go/adapters/messaging/messaging.go`](../../go/adapters/messaging/messaging.go)
- [`go/examples/messaging/main.go`](../../go/examples/messaging/main.go)
- [`go/examples/messaging/main_test.go`](../../go/examples/messaging/main_test.go)
- [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)
- [`typescript/src/reference-messaging.ts`](../../typescript/src/reference-messaging.ts)
- [`typescript/src/reference-messaging.test.ts`](../../typescript/src/reference-messaging.test.ts)
- [`typescript/src/messaging-example.ts`](../../typescript/src/messaging-example.ts)

## Related

- [Messaging middleware should own context and metadata at the consumer boundary](013-messaging-boundary-middleware.md)
- [The HTTP reference vertical should demonstrate application-owned trust policy](051-http-reference-trust-policy.md)
- [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
