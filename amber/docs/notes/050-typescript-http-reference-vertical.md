# The TypeScript HTTP reference vertical should match the Go boundary

Both SDKs should demonstrate the same application-level provenance behavior at
their HTTP boundaries, while leaving runtime and persistence ownership with
the application.

## Origin

Amber had a runnable TypeScript Fetch-compatible composition example, but the
service behavior was inline and did not verify persistence or malformed-input
rejection. The TypeScript middleware also overwrote an explicitly supplied
child response header with the inbound parent header, unlike the Go boundary.

## What

The TypeScript example now uses `createReferenceServiceHandler`, which:

- accepts an inbound provenance value or creates a local root for a top-level
  request;
- derives and stores an `OriginIncoming` child through the application-owned
  `ProvenanceStore`; and
- explicitly returns the child provenance in the response header.

The HTTP middleware preserves an existing response provenance header and only
applies the inbound value when the application has not selected one. Focused
tests cover accepted, absent, and malformed input.

## Why

This keeps the Go and TypeScript reference paths semantically aligned without
introducing a framework-specific Node server or a mandatory storage runtime.
Fetch-compatible applications can provide their own server, worker, or
framework adapter and their own durable `ProvenanceStore`.

## Used in

- [`typescript/src/reference-service.ts`](../../typescript/src/reference-service.ts)
- [`typescript/src/reference-service.test.ts`](../../typescript/src/reference-service.test.ts)
- [`typescript/src/http.ts`](../../typescript/src/http.ts)
- [`typescript/src/example.ts`](../../typescript/src/example.ts)

## Related

- [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
- [The Go HTTP reference vertical should have focused boundary tests](049-go-http-reference-vertical-tests.md)
