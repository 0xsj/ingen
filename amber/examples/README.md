# Amber examples

These examples compose the current adapters into one small request flow:

1. start a root provenance value;
2. put it on an outgoing HTTP request;
3. install it through HTTP middleware;
4. derive an incoming child execution;
5. persist the child; and
6. project the child into logging and tracing fields.

Run both examples from the project root:

```sh
make examples
```

The Go example uses a temporary file store and removes that file when it exits.
The TypeScript example uses the in-memory key-value backend to keep the example
portable across Node and browser-oriented runtimes.

The examples use `httptest`/`Request` objects rather than opening a network
port, so they demonstrate adapter composition without external services.

## Implementations

- Go: [`go/examples/compose`](../go/examples/compose/)
- TypeScript: [`typescript/src/example.ts`](../typescript/src/example.ts)
