# The Go HTTP reference vertical should have focused boundary tests

The runnable service example demonstrates composition; focused tests should
protect the application boundary as the example evolves.

## Origin

The Go `net/http` reference vertical had an end-to-end executable path, but
its accepted inbound flow and malformed-input rejection were only verified by
running the example or by lower-level HTTP adapter tests.

## What

The service handler is now constructed by `newServiceHandler`, allowing the
application boundary to be exercised with an in-memory `Store`. Focused tests
cover:

- accepted inbound provenance being converted into an `OriginIncoming` child,
  persisted, and returned in the response;
- malformed inbound provenance being rejected with HTTP 400 before the
  application handler responds.

The runnable example continues to use a temporary file store and a real local
listener, so the focused tests do not replace the composition proof.

## Why

This separates fast behavioral regression coverage from the executable
deployment example while preserving the same handler construction. It also
makes the trust boundary explicit: malformed input does not reach application
logic or produce an outgoing provenance value.

## Example

Run the focused reference-service boundary tests with:

```sh
cd go
go test ./examples/service -count=1
```

The tests cover accepted inbound values and malformed-input rejection without
starting the listener used by the runnable example.

## Gotchas

- These tests protect the reference boundary; they do not make the reference
  service a production server template.
- Storage, routing, and trust policy remain application-owned choices.

## Used in

- [`go/examples/service/main.go`](../../go/examples/service/main.go)
- [`go/examples/service/main_test.go`](../../go/examples/service/main_test.go)
- [`go/adapters/http/http.go`](../../go/adapters/http/http.go)

## Related

- [The first deployment vertical should prove a real Go HTTP service path](047-go-http-reference-vertical.md)
- [HTTP middleware should own the request and response provenance boundary](012-http-boundary-middleware.md)
