# Public contract-test helpers should pass an external consumer check

An adapter test helper is only useful to downstream users if its public import
boundary works outside Amber's own package tree.

## Origin

The reusable Go `contracttest` package and TypeScript `@0xsj/amber/testing`
subpath were tested inside the repository. The TypeScript package smoke test
already installed and exercised its subpath, but the external Go module smoke
test only compiled the runtime consumer and did not import the new helper.

## What

The external Go consumer now includes a test that imports
`adapters/storage/contracttest` and runs `RunStoreContract` against the public
`amberstorage.Store` boundary. `make module-check` runs that test before the
consumer program, while the TypeScript package smoke test exercises the
installed testing subpath.

## Why

This closes the distinction between an internal package test and a usable
downstream contract. It catches module path, package visibility, and dependency
boundary mistakes before release without requiring a published module.

## Example

The external test uses the same shape a downstream Go adapter package can use:

```go
contracttest.RunStoreContract(t, func(testing.TB) amberstorage.Store {
		return amberstorage.NewMemoryStore()
})
```

The temporary module uses a local `replace` directive, so the check remains
deterministic and does not publish or download Amber.

## Gotchas

- The external check proves importability and helper behavior, not a published
  Go proxy version.
- A custom backend still needs its own implementation-specific tests for
  durability, transactions, and operational behavior.
- The helper remains opt-in and does not enter the main runtime API.

## Used in

- [`go/examples/consumer/contract_test.go`](../../go/examples/consumer/contract_test.go)
- [`go/adapters/storage/contracttest`](../../go/adapters/storage/contracttest/)
- [`Makefile`](../../Makefile)
- [`typescript/test/package-smoke.mjs`](../../typescript/test/package-smoke.mjs)
- [`docs/adapter-authoring.md`](../adapter-authoring.md)

## Related

- [Custom storage adapters should have reusable contract-test helpers](068-reusable-storage-contract-helpers.md)
- [The Go module should pass a clean consumer smoke test](027-go-module-consumer-smoke-test.md)
- [The package artifact should pass a clean consumer smoke test](026-package-install-smoke-test.md)
- [The optional PostgreSQL import path should have an explicit compatibility check](043-postgres-public-api-boundary.md)
