# HTTP middleware should own the request and response provenance boundary

The HTTP adapter becomes directly usable when one middleware entry point
handles incoming context, application execution, and outgoing response
propagation.

## Origin

The lower-level HTTP helpers already encoded and decoded the
`Amber-Provenance` header, but each application would otherwise need to repeat
the same request-context and response-header wiring.

## What

The Go adapter provides `Middleware` and
`MiddlewareWithErrorHandler` for `net/http`. The middleware derives the
request context from the incoming header, adds the accepted value to the
response header, and invokes the next handler. Rejected malformed input stops
the chain with `400 Bad Request` by default; ignored malformed input continues
without installing provenance.

The TypeScript adapter provides `middleware` for Fetch-compatible handlers. A
wrapped handler receives `(request, context)` and returns a `Response`; the
middleware returns a response carrying the context provenance when present.
`withOutgoingResponse` performs the response clone so the original response is
not mutated.

## Why

This keeps the propagation boundary consistent while leaving framework routing,
authorization, handler errors, and provenance creation to the application. A
missing incoming value remains missing, so middleware does not silently invent
an execution identity.

## Example

```go
handler := amberhttp.Middleware(applicationHandler, amber.IncomingReject)
http.ListenAndServe(":8080", handler)
```

```ts
const handler = middleware(async (_request, context) => {
  return new Response(context.provenance ? "tracked" : "untracked");
}, "reject");
const response = await handler(request, ProvenanceContext.empty());
```

## Gotchas

- The default reject behavior is a generic 400 response; use the custom error
  handler when the application needs a different response shape.
- Middleware does not create provenance when the incoming header is absent.
- TypeScript response propagation rebuilds the `Response`, so the handler must
  return an unconsumed response body.
- A handler that deliberately deletes or replaces the response header can
  override the value after middleware has installed it.

## Used in

- [`go/adapters/http/http.go`](../../go/adapters/http/http.go)
- [`go/adapters/http/http_test.go`](../../go/adapters/http/http_test.go)
- [`typescript/src/http.ts`](../../typescript/src/http.ts)
- [`typescript/src/http.test.ts`](../../typescript/src/http.test.ts)
- [`adapters/http/README.md`](../../adapters/http/README.md)

## Related

- [The HTTP adapter should preserve the core JSON inside a transport-safe envelope](006-http-header-adapter.md)
- [Incoming handling must make the failure policy explicit](005-explicit-incoming-policy.md)
- [A project needs one verification command that crosses its language boundaries](011-root-verification-command.md)
