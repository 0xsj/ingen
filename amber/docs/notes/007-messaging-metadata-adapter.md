# Message metadata should reuse the transport envelope without sharing mutable maps

Messaging propagation can share the Amber transport value format with HTTP
while keeping message metadata ownership explicit and immutable at the adapter
boundary.

## Origin

The second transport adapter needed to carry provenance through broker-neutral
metadata without coupling the messaging package to `net/http` or to any one
message broker.

## What

The messaging adapter uses the same `Amber-Provenance` key and unpadded
base64url-encoded canonical JSON as HTTP. Go accepts a string map and returns a
cloned map with provenance added; TypeScript accepts a readonly string record
and returns a new record. Incoming helpers inspect metadata using the core
policy and install accepted values into a derived context.

The encoding logic now lives in a transport-neutral helper, so HTTP and
messaging do not maintain separate base64 implementations.

## Why

A broker-specific envelope would duplicate the core wire contract and make
cross-transport messages harder to inspect. Mutating a caller's metadata map
would also let a reused message carry stale provenance into a later send. A
shared value envelope plus cloned metadata keeps the semantic contract common
while leaving delivery, ordering, and retry behavior to the broker adapter.

## Example

```text
message metadata -> Amber-Provenance: <base64url JSON>
                 -> inspect with policy
                 -> derive context for handler
outgoing metadata <- clone + new provenance value
```

## Gotchas

- The metadata key is exact and case-sensitive in the broker-neutral map; a
  broker adapter may need a separate normalization layer.
- Message delivery retries are not automatically Amber retries; the consumer
  must decide when to call the core `Retry` transition.
- The shared envelope does not prove sender identity or message authenticity.
- Broker-specific headers, body envelopes, acknowledgement semantics, and
  dead-letter behavior are still future adapter work.

## Used in

- [`adapters/messaging/README.md`](../../adapters/messaging/README.md)
- [`go/adapters/transport/transport.go`](../../go/adapters/transport/transport.go)
- [`go/adapters/messaging/messaging.go`](../../go/adapters/messaging/messaging.go)
- [`go/adapters/messaging/messaging_test.go`](../../go/adapters/messaging/messaging_test.go)
- [`typescript/src/transport.ts`](../../typescript/src/transport.ts)
- [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)
- [`typescript/src/messaging.test.ts`](../../typescript/src/messaging.test.ts)
- [`conformance/messaging-v1.json`](../../conformance/messaging-v1.json)

## Related

- [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
