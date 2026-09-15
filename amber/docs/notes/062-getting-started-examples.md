# Getting-started snippets should be executable so the first-use path cannot drift

Documentation examples are most useful when they are compiled and run by the
same checks as the implementation. Otherwise a small API rename can leave the
first-use guide looking correct while new users receive code that does not run.

## Origin

The cross-language getting-started guide showed minimal Go and TypeScript root,
context, and child flows, but those snippets were documentation-only. The
repository already had a runnable examples gate that could verify them.

## What

The guide's minimal flows are now executable as:

- `go/examples/getting-started/main.go`, run by
  `make example-go-getting-started`;
- `typescript/src/getting-started-example.ts`, run by
  `make example-typescript-getting-started` and its npm script.

Both targets are included in `make examples`, so `make release-check` also
protects the first-use path.

## Why

This keeps the adoption guide close to the public SDK APIs and gives both
language implementations the same smallest useful example. The examples do
not introduce a framework, database, network service, or telemetry dependency.

## Example

Run the two focused examples from the project root:

```sh
make example-go-getting-started
make example-typescript-getting-started
```

Each prints the root and child execution IDs after installing the root value
into the SDK context abstraction and deriving an incoming child.

## Gotchas

- The examples demonstrate context composition, not HTTP or messaging
  propagation; those remain in the adapter examples.
- Generated TypeScript output is a build artifact; the source example remains
  under `typescript/src/`.
- The examples use random IDs, so output identifiers are expected to differ on
  every run.

## Used in

- [`go/examples/getting-started/main.go`](../../go/examples/getting-started/main.go)
- [`typescript/src/getting-started-example.ts`](../../typescript/src/getting-started-example.ts)
- [`Makefile`](../../Makefile)
- [`docs/getting-started.md`](../getting-started.md)
- [`examples/README.md`](../../examples/README.md)

## Related

- [Getting started guidance should show the smallest useful cross-language path](061-getting-started-guide.md)
- [A runnable composition example should cross the adapters without external services](015-end-to-end-composition-example.md)
- [The release-candidate gate should run every runnable example](033-release-candidate-gate.md)
