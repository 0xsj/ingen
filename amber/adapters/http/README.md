# HTTP adapter

The Amber HTTP adapter carries one provenance value in the
`Amber-Provenance` header.

## Wire format

The header value is unpadded base64url encoding of the UTF-8 canonical JSON
representation defined by [`spec/v1.md`](../../spec/v1.md). The adapter accepts
no alternate base64 alphabet or padding form. Missing and empty headers are
absent. Non-empty malformed headers follow the caller's `reject` or `ignore`
policy.

The decoded JSON payload is limited to `16,384` bytes, matching the core
incoming boundary. The encoded header is bounded before decoding as well. The
adapter never installs a decoded value implicitly; callers choose the context
to derive, and outgoing request helpers clone rather than mutate the source
request.

## Implementations

- Go: [`go/adapters/http`](../../go/adapters/http/)
- TypeScript: [`typescript/src/http.ts`](../../typescript/src/http.ts)

The Go adapter integrates with `net/http`. The TypeScript adapter uses standard
`Headers` and `Request` objects so it can be used with Fetch-compatible runtimes
without coupling the core SDK to a web framework.

Both implementations expose validator variants (`DecodeHeaderWithValidator`
and `httpMiddlewareWithValidator`, with equivalent TypeScript helpers). The
validator runs after structural decoding and before context installation.
`IncomingReject` returns a validation error; `IncomingIgnore` treats a failed
validation as absent. These hooks do not define a signature format—applications
must authenticate a transport envelope separately if cryptographic authenticity
is required.

## Middleware

The Go adapter exposes `Middleware` for `net/http`. It derives the request
context from the incoming header, then adds the accepted context value to the
response header before invoking the application handler. Rejected input returns
`400 Bad Request` by default; `MiddlewareWithErrorHandler` allows an
application-defined error response. `IncomingIgnore` continues without
installing malformed input.

The TypeScript adapter exposes `httpMiddleware`, which wraps a Fetch-compatible
handler of `(request, context) => response`. It returns a response carrying the
accepted provenance, or a generic `400` response when `reject` encounters
malformed input. The error handler can be supplied as the third argument.

Neither implementation creates provenance automatically. A missing incoming
header remains missing unless the caller supplies provenance in the existing
request context.

The encoded-header compatibility vector is kept in
[`conformance/http-v1.json`](../../conformance/http-v1.json) and is consumed by
both adapter test suites.
