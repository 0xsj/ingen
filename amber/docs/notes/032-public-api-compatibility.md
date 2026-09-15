# The public API should change only through an explicit compatibility check

Release readiness includes knowing what consumers can import, not only whether
the implementation tests pass.

## Origin

Amber now has several adapters and a TypeScript package artifact. The existing
smoke tests exercised representative functions, but a new export could still
silently widen the package surface or a removed export could go unnoticed.

## What

The TypeScript SDK now has a sorted public-export manifest at
[`typescript/test/public-api.json`](../../typescript/test/public-api.json).
The TypeScript test suite compares the built entry point against that manifest,
and the package tarball smoke test performs the same comparison after installing
the actual artifact. The Go external consumer smoke test now imports and
exercises the root package plus HTTP, transport, messaging, logging, tracing,
OpenTelemetry, and storage adapter boundaries.

## Why

An explicit manifest makes public API additions and removals reviewable. The
external Go consumer catches module-boundary problems, while the packaged
TypeScript check catches export-map and artifact-boundary problems. Together
they protect consumers without freezing private implementation details.

## Example

```sh
make check
```

The TypeScript suite reports `TypeScript public API manifest passed`, and the
external Go consumer reports that all public adapter boundaries are present.

## Gotchas

- Adding an intentional TypeScript public export requires updating the manifest
  in the same change.
- The manifest checks runtime exports; TypeScript-only type exports are checked
  by `tsc` and are not present in JavaScript's `Object.keys` result.
- The Go consumer is a boundary smoke test, not a complete API inventory; Go
  package documentation and package tests remain authoritative for details.

## Used in

- [`typescript/test/public-api.json`](../../typescript/test/public-api.json)
- [`typescript/test/public-api.mjs`](../../typescript/test/public-api.mjs)
- [`typescript/test/package-smoke.mjs`](../../typescript/test/package-smoke.mjs)
- [`go/examples/consumer/main.go`](../../go/examples/consumer/main.go)
- [`Makefile`](../../Makefile)

## Related

- [The TypeScript package should publish one explicit, testable entry artifact](021-package-artifact-contract.md)
- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
- [The Go module should pass a clean consumer smoke test](027-go-module-consumer-smoke-test.md)
- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
