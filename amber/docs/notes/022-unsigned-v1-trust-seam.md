# Amber v1 should stay unsigned by default while exposing an explicit trust seam

Structural validation and authenticity are different decisions, so the core
library should not silently make one stand in for the other.

## Origin

Amber's incoming boundaries validated IDs, enums, sizes, and JSON shape, but a
valid unsigned value could still be supplied by an untrusted sender. Adding a
signature scheme now would also force choices about algorithms, key IDs,
rotation, replay protection, and asynchronous key lookup that are not shared
by every deployment.

## What

The v1 contract is explicitly unsigned by default. Both SDKs expose an optional
`IncomingValidator` that runs after structural decoding and before context
installation. HTTP and messaging adapters expose validator-aware decode,
context, request/message, and middleware helpers. Validator failures follow the
existing incoming policy: `reject` returns an error, while `ignore` treats the
value as absent.

The current validator hook is synchronous and receives the decoded provenance.
It is suitable for allowlists or already-established deployment trust. A
cryptographically authenticated transport must verify its raw envelope through
its own protocol or a future separately versioned adapter contract.

## Why

This keeps the default wire format portable and preserves backward-compatible
adoption while preventing applications from confusing “valid Amber shape” with
“trusted sender.” It also gives downstream context consumers a clear boundary:
only values accepted by the configured trust rule enter the context.

## Example

```go
validator := func(value amber.Provenance) error {
    if value.Origin() != amber.OriginIncoming {
        return errors.New("unexpected origin")
    }
    return nil
}
handler := amberhttp.MiddlewareWithValidator(next, amber.IncomingReject, validator)
```

```ts
const validator = (value: Provenance): void => {
  if (value.origin !== "incoming") throw new Error("unexpected origin");
};
const handler = httpMiddlewareWithValidator(next, "reject", validator);
```

## Gotchas

- A validator is not an authorization system; normal authentication and
  authorization checks remain required.
- IDs, attribution, tenant fields, and references remain untrusted unless the
  application independently accepts them.
- `ignore` can hide rejected trust input, so use `reject` when provenance is a
  required trusted workflow input.
- The hook does not receive a signature or raw header/metadata bytes. Signed
  envelopes need an explicit authenticated adapter design.

## Used in

- [`spec/trust-v1.md`](../../spec/trust-v1.md)
- [`spec/v1.md`](../../spec/v1.md)
- [`go/incoming.go`](../../go/incoming.go)
- [`go/adapters/http/http.go`](../../go/adapters/http/http.go)
- [`go/adapters/messaging/messaging.go`](../../go/adapters/messaging/messaging.go)
- [`typescript/src/incoming.ts`](../../typescript/src/incoming.ts)
- [`typescript/src/http.ts`](../../typescript/src/http.ts)
- [`typescript/src/messaging.ts`](../../typescript/src/messaging.ts)

## Related

- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
- [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
- [Message metadata should reuse the transport envelope without sharing mutable maps](007-messaging-metadata-adapter.md)
- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
