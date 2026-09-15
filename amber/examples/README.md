# Amber examples

These examples compose the current adapters into one small request flow:

1. start a root provenance value;
2. put it on an outgoing HTTP request;
3. install it through HTTP middleware;
4. derive an incoming child execution;
5. persist the child; and
6. project the child into logging and tracing fields.

Run all examples from the project root:

```sh
make examples
```

The Go example uses a temporary file store and removes that file when it exits.
The TypeScript example uses the in-memory key-value backend to keep the example
portable across Node and browser-oriented runtimes.

The examples use `httptest`/`Request` objects rather than opening a network
port, so they demonstrate adapter composition without external services.

The smallest root/context/child walkthroughs are available as
[`go/examples/getting-started`](../go/examples/getting-started/) and
[`typescript/src/getting-started-example.ts`](../typescript/src/getting-started-example.ts).
Run them individually with `make example-go-getting-started` and
`make example-typescript-getting-started`, or use `make examples` for the full
set.

The OpenTelemetry examples separately show application-owned tracer setup,
Amber span enrichment, and finished-span inspection using in-memory SDK
recorders. They do not require a collector or network destination.

## Implementations

- Go: [`go/examples/compose`](../go/examples/compose/)
- TypeScript: [`typescript/src/example.ts`](../typescript/src/example.ts)
- Go OpenTelemetry: [`go/examples/otel`](../go/examples/otel/)
- TypeScript OpenTelemetry: [`typescript/src/otel-example.ts`](../typescript/src/otel-example.ts)
