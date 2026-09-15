# The HTTP reference vertical should demonstrate application-owned trust policy

Amber should show where a deployment-specific trust decision is attached
without implying that provenance itself provides authentication or
authorization.

## Origin

The Go and TypeScript reference handlers demonstrated structural decoding,
child derivation, storage, and response propagation. Trust-aware middleware
already existed in both SDKs, but the reference vertical had not shown the
validator being applied before application logic.

## What

Both reference handlers now accept an optional application-owned incoming
validator. The validator runs at the HTTP boundary through the existing
`MiddlewareWithValidator` / `httpMiddlewareWithValidator` seams:

- a trusted inbound value reaches the handler and produces a stored child;
- an untrusted but structurally valid value receives HTTP 400; and
- the untrusted request produces no stored child.

The default reference path remains validator-free and structurally rejecting,
so existing applications are not forced to select a trust policy.

## Why

This makes the trust boundary operationally visible while preserving Amber's
v1 scope. Applications still own authentication, authorization, allowlists,
key management, and any cryptographic envelope verification.

## Example

Provide a validator at the reference HTTP boundary when the application needs
trust decisions beyond structural validation:

```go
server := amberhttp.MiddlewareWithValidator(
    application,
    amber.IncomingReject,
    validateIncoming,
)
```

The TypeScript reference handler exposes the corresponding validator hook.

## Gotchas

- A validator is application-owned policy; Amber does not authenticate or
  authorize the caller.
- The default path remains structurally validating and unsigned.

## Used in

- [`go/examples/service/main.go`](../../go/examples/service/main.go)
- [`go/examples/service/main_test.go`](../../go/examples/service/main_test.go)
- [`typescript/src/reference-service.ts`](../../typescript/src/reference-service.ts)
- [`typescript/src/reference-service.test.ts`](../../typescript/src/reference-service.test.ts)
- [`spec/trust-v1.md`](../../spec/trust-v1.md)

## Related

- [Amber v1 should stay unsigned by default while exposing an explicit trust seam](022-unsigned-v1-trust-seam.md)
- [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
- [The TypeScript HTTP reference vertical should match the Go boundary](050-typescript-http-reference-vertical.md)
