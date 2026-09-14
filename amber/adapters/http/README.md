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

The encoded-header compatibility vector is kept in
[`conformance/http-v1.json`](../../conformance/http-v1.json) and is consumed by
both adapter test suites.
