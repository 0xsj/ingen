# The first deployment vertical should prove a real Go HTTP service path

After the foundation checkpoint, one concrete vertical is more useful than
another generic abstraction.

## Origin

Amber's HTTP adapter and file store were individually tested, and the
composition example connected them through `httptest`. The project did not yet
show a real local `net/http` listener receiving propagated provenance, deriving
a request execution, persisting it through the injected `Store`, and returning
provenance on the response.

## What

The Go `examples/service` program is the first reference vertical. It:

1. injects a file-backed `Store` into an HTTP handler;
2. starts a real loopback `net/http` listener;
3. sends a request with an Amber provenance header;
4. applies the HTTP middleware and derives an incoming child execution;
5. persists the child and returns its execution ID; and
6. verifies the response header and stored value before clean shutdown.

`make examples` runs this service alongside the existing composition and
OpenTelemetry examples. The example uses the file store and has no external
database, broker, or collector dependency.

## Why

This establishes the integration shape applications can copy while preserving
the architectural boundaries: the HTTP adapter owns boundary propagation, the
application owns the handler and transition choice, and the injected store owns
persistence. PostgreSQL can replace the file store without changing the HTTP
boundary.

## Gotchas

- The example is an executable reference flow, not a production server
  template; production deployments still choose routing, timeouts, logging,
  graceful-shutdown policy, and storage transactions.
- Missing inbound provenance is handled by explicitly creating a local root;
  this policy belongs to the application, not the HTTP adapter.
- The example binds only to loopback and shuts down before exiting.

## Used in

- [`go/examples/service/main.go`](../../go/examples/service/main.go)
- [`Makefile`](../../Makefile)
- [`go/adapters/http/http.go`](../../go/adapters/http/http.go)
- [`go/adapters/storage/storage.go`](../../go/adapters/storage/storage.go)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
- [HTTP middleware should own the request and response provenance boundary](012-http-boundary-middleware.md)
- [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
