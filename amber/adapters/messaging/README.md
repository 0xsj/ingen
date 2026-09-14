# Messaging adapter

The Amber messaging adapter carries provenance in string-valued message
metadata under the `Amber-Provenance` key. The value uses the same unpadded
base64url envelope as the HTTP adapter; broker-specific message bodies and
delivery semantics remain outside this package.

Incoming metadata is inspected with the caller's `reject` or `ignore` policy
and can be installed into a derived context explicitly. Outgoing metadata is
returned as a clone so adding provenance cannot mutate a reusable message
metadata map.

## Implementations

- Go: [`go/adapters/messaging`](../../go/adapters/messaging/)
- TypeScript: [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)
- Shared transport encoding: [`go/adapters/transport`](../../go/adapters/transport/)
  and [`typescript/src/transport.ts`](../../typescript/src/transport.ts)

The metadata compatibility vector is kept in
[`conformance/messaging-v1.json`](../../conformance/messaging-v1.json).
