# The HTTP adapter should preserve the core JSON inside a transport-safe envelope

The HTTP adapter should encode the canonical provenance JSON rather than invent
a second HTTP-specific provenance model.

## Origin

The first transport adapter needed a header representation that was safe for
HTTP field values, bounded before decoding, and identical across Go and
TypeScript.

## What

Both adapters use the `Amber-Provenance` header. Its value is unpadded base64url
of the UTF-8 canonical JSON defined by the core specification. The adapter
checks the encoded size before decoding, delegates decoded JSON validation and
`reject`/`ignore` policy to the core, and provides helpers for incoming headers
and outgoing request propagation.

The Go request helper returns a cloned `net/http` request. The TypeScript helper
returns a cloned Fetch `Request`. Neither mutates the source request.

## Why

Putting raw JSON in a header would expose quoting and intermediary handling
differences, while defining separate header fields would duplicate the core
model and create another place for Go and TypeScript to drift. Base64url keeps
the field value transport-safe, and retaining the canonical JSON as the payload
keeps validation and semantics in one layer.

## Example

```text
Provenance -> canonical UTF-8 JSON -> unpadded base64url
          -> Amber-Provenance header -> decode -> core validation -> context
```

## Gotchas

- Base64 expands the payload, so the adapter bounds the encoded header before
  decoding and the core bounds the decoded JSON.
- The header is a propagation mechanism, not proof that the sender is trusted;
  callers still choose the incoming policy and attribution remains descriptive.
- Outgoing helpers clone requests because mutating a caller's request can leak
  one operation's provenance into a reused request.
- HTTP is the first adapter; message, logging, tracing, and persistence formats
  may have different envelope constraints.

## Used in

- [`adapters/http/README.md`](../../adapters/http/README.md)
- [`go/adapters/http/http.go`](../../go/adapters/http/http.go)
- [`go/adapters/http/http_test.go`](../../go/adapters/http/http_test.go)
- [`typescript/src/http.ts`](../../typescript/src/http.ts)
- [`typescript/src/http.test.ts`](../../typescript/src/http.test.ts)
- [`conformance/http-v1.json`](../../conformance/http-v1.json)

## Related

- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
- [`spec/v1.md`](../../spec/v1.md)
