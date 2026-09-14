# Messaging adapter

The Amber messaging adapter carries provenance in string-valued message
metadata under the `Amber-Provenance` key. The value uses the same unpadded
base64url envelope as the HTTP adapter; broker-specific message bodies and
delivery semantics remain outside this package.

Incoming metadata is inspected with the caller's `reject` or `ignore` policy
and can be installed into a derived context explicitly. Outgoing metadata is
returned as a clone so adding provenance cannot mutate a reusable message
metadata map.

## Middleware

The Go adapter provides a generic `Message[T]` and `Middleware` wrapper. It
clones incoming metadata, derives the consumer context, and adds the accepted
provenance to the returned message metadata. Rejected malformed metadata
returns an error before the consumer runs; ignored malformed metadata continues
without installing provenance.

The TypeScript adapter provides generic `Message<T>` helpers and
`messageMiddleware` for handlers of `(message, context) => message`. Metadata
is cloned at the boundary, while the message body remains opaque and unchanged
by Amber.

These wrappers do not implement broker delivery, acknowledgement, retry, or
dead-letter behavior. A consumer decides when a delivery attempt becomes an
Amber `Retry` transition.

Validator variants (`DecodeMetadataWithValidator`,
`MiddlewareWithValidator`, and their TypeScript equivalents) run after
structural decoding and before context installation. They are opt-in trust
hooks, not authorization or signature protocols. Under `IncomingReject`, a
failed validation returns an error; under `IncomingIgnore`, it is treated as
absent.

## Implementations

- Go: [`go/adapters/messaging`](../../go/adapters/messaging/)
- TypeScript: [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)
- Shared transport encoding: [`go/adapters/transport`](../../go/adapters/transport/)
  and [`typescript/src/transport.ts`](../../typescript/src/transport.ts)

The metadata compatibility vector is kept in
[`conformance/messaging-v1.json`](../../conformance/messaging-v1.json).
