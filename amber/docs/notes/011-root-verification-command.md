# A project needs one verification command that crosses its language boundaries

When a repository contains multiple SDKs, a package-local green test suite is
not enough; the root workflow must exercise each implementation and the shared
fixtures together.

## Origin

Amber's Go and TypeScript suites were independently runnable from their module
directories, but the project had no single command a contributor could use to
verify the whole implementation.

## What

The root `Makefile` provides `make test`, `make lint`, and `make check`. The
test target runs `go test ./...` inside `go/` and `npm test` inside
`typescript/`, which includes the core, context, HTTP, messaging, logging,
tracing, storage, and shared conformance checks. The lint target runs `go vet
./...` and `npm run typecheck`; check combines both.

## Why

Leaving the commands separate makes it easy to validate only the language a
contributor changed and miss a wire-contract regression in the other SDK. A
small root target gives the repository one repeatable verification entry point
without hiding the package-local commands used for focused work.

## Example

```sh
make test    # full Go + TypeScript + conformance verification
make lint    # static checks without running the test suites
make check   # static checks plus the full test suite
```

## Gotchas

- `make test` expects the TypeScript dependencies to have been installed with
  `npm install` or `npm ci`.
- `make lint-typescript` also expects the TypeScript dependencies to be
  installed.
- The Go suite uses the module under `go/`; the root is not itself a Go module.
- A passing root test establishes compatibility with the current fixtures; it
  does not prove transport security, storage durability, or application-level
  correctness.

## Used in

- [`Makefile`](../../Makefile)
- [`README.md`](../../README.md)
- [`go/`](../../go/)
- [`typescript/package.json`](../../typescript/package.json)

## Related

- [Conformance starts with shared invalid values](004-shared-conformance-fixtures.md)
- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
