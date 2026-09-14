# Messaging middleware should own context and metadata at the consumer boundary

The messaging adapter becomes directly usable when one broker-neutral wrapper
handles metadata inspection, consumer context, and outgoing metadata without
claiming ownership of delivery semantics.

## Origin

The lower-level messaging helpers already encoded and decoded the
`Amber-Provenance` metadata value, but a consumer still needed to repeat the
same context installation and outgoing metadata cloning around every handler.

## What

The Go adapter provides generic `Message[T]`, `Handler[T]`, and `Middleware`
types. Middleware clones incoming metadata, derives the consumer context, and
adds the accepted provenance to returned message metadata. Rejected malformed
metadata returns an error before the consumer runs; ignored malformed metadata
continues without installing provenance.

The TypeScript adapter provides generic `Message<T>` helpers and
`messageMiddleware` for handlers of `(message, context) => message`. Amber
treats the body as opaque and only owns the metadata map at the propagation
boundary.

## Why

Message brokers differ in payload types, acknowledgement APIs, delivery
attempts, and dead-letter behavior. A small generic boundary removes repeated
provenance wiring without pretending those broker-specific concerns have one
portable implementation.

## Example

```go
handler := ambermessaging.Middleware[string](consume, amber.IncomingReject)
outgoing, err := handler(ctx, ambermessaging.Message[string]{
    Body: "input",
    Metadata: metadata,
})
```

```ts
const handler = messageMiddleware(async (message, context) => ({
  body: `${message.body}-handled`,
  metadata: message.metadata,
}), "reject");
const outgoing = await handler(message, ProvenanceContext.empty());
```

## Gotchas

- Rejected metadata returns an error in both SDKs; there is no HTTP-style
  status response at the broker-neutral layer.
- Middleware does not create provenance when both incoming metadata and the
  supplied context are absent.
- The message body is opaque; Amber does not clone, serialize, or authenticate
  it.
- Delivery retries are not automatically Amber `Retry` transitions. The
  consumer must make that decision explicitly.

## Used in

- [`adapters/messaging/README.md`](../../adapters/messaging/README.md)
- [`go/adapters/messaging/messaging.go`](../../go/adapters/messaging/messaging.go)
- [`go/adapters/messaging/messaging_test.go`](../../go/adapters/messaging/messaging_test.go)
- [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)
- [`typescript/src/messaging.test.ts`](../../typescript/src/messaging.test.ts)

## Related

- [Message metadata should reuse the transport envelope without sharing mutable maps](007-messaging-metadata-adapter.md)
- [HTTP middleware should own the request and response provenance boundary](012-http-boundary-middleware.md)
- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
