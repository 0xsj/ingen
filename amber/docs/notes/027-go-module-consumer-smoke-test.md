# The Go module should pass a clean consumer smoke test

Go consumers need the published module path and exported package boundaries to
work outside Amber's own module tree.

## Origin

Amber's Go tests and composition example imported the module's packages, but
they ran inside the Amber module itself. That does not fully exercise how a
separate application resolves the module path.

## What

`make module-check` copies a small consumer program into a temporary directory,
initializes a separate Go module, replaces the public Amber module path with the
local checkout, runs `go mod tidy`, and executes the consumer. The program
creates provenance, derives an incoming child, and uses the public HTTP adapter
to create an outgoing request with an Amber header.

## Why

This catches incorrect module paths, missing exported packages, and consumer
compile or runtime failures that package-local tests can overlook. The local
`replace` keeps the check deterministic and avoids publishing or downloading
Amber from a registry.

## Example

```sh
make module-check
```

The full `make check` gate includes this smoke test.

## Gotchas

- The check validates the local source through the public module path; it does
  not verify a version already published to a Go proxy.
- The temporary consumer only exercises a small public surface. Package tests
  remain responsible for detailed behavior and conformance.
- The Go module currently has no external runtime dependencies.

## Used in

- [`Makefile`](../../Makefile)
- [`go/examples/consumer/main.go`](../../go/examples/consumer/main.go)
- [`go/go.mod`](../../go/go.mod)
- [`README.md`](../../README.md)

## Related

- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
