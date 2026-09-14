# A runnable composition example should cross the adapters without external services

Examples are part of the implementation record when they exercise the same
boundaries a real application will compose.

## Origin

Amber had individually tested model, context, transport, observability, and
storage adapters, but a new contributor still had to infer how those pieces fit
together in an application flow.

## What

The Go and TypeScript examples start root provenance, propagate it over HTTP,
install it through middleware, derive an incoming child execution, persist that
child, and project it into logging and tracing fields. `make examples` runs both
without opening a network port or requiring an external service.

The Go example uses a temporary file store. The TypeScript example uses the
portable in-memory key-value backend while keeping the same `KeyValueStore`
interface that a durable runtime backend would implement.

## Why

An end-to-end composition catches mismatched assumptions between adapters that
unit tests can miss, while a no-network example remains easy to run from a
fresh checkout. Keeping the child derivation explicit also demonstrates that
Amber does not invent application execution semantics inside transport code.

## Example

```sh
make examples
```

The resulting output includes the response status, whether the Amber response
header was propagated, and the stored child projection.

## Gotchas

- The examples are composition checks, not a production HTTP server or a
  database benchmark.
- HTTP middleware propagates the context value it receives; the application
  explicitly derives the child execution inside the handler.
- The TypeScript example's in-memory backend demonstrates the interface, not
  process restart durability.
- Go's temporary file is intentionally deleted when the example exits.

## Used in

- [`examples/README.md`](../../examples/README.md)
- [`go/examples/compose/main.go`](../../go/examples/compose/main.go)
- [`typescript/src/example.ts`](../../typescript/src/example.ts)
- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)

## Related

- [HTTP middleware should own the request and response provenance boundary](012-http-boundary-middleware.md)
- [Messaging middleware should own context and metadata at the consumer boundary](013-messaging-boundary-middleware.md)
- [Durable storage should preserve append-only semantics behind an explicit backend seam](014-durable-storage-backend-seam.md)
