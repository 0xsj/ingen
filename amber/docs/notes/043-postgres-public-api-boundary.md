# The optional PostgreSQL import path should have an explicit compatibility check

Vendor-specific integrations are still public APIs when applications import
them directly, so their boundary needs a focused compatibility check.

## Origin

Amber exposed PostgreSQL through the explicit `amberpostgres` package path while
retaining the original parent-package implementation for v1 compatibility. The
external consumer smoke test proved that the package could be imported, but it
did not pin the documented schema constants, readiness method, or error
sentinel.

## What

The optional package now has an external-package test that verifies:

- `PostgresStore` still satisfies the generic `Store` contract;
- `CheckSchema(context.Context)` remains available;
- `PostgresSchema`, `PostgresSchemaName`, and `PostgresSchemaVersion` remain
  coherent and public; and
- `ErrUnsupportedSchemaVersion` remains the same sentinel as the generic
  storage package.

The test also confirms the constructor continues to reject a nil database with
the core invalid-transition error.

## Why

An alias-based compatibility boundary is easy to accidentally break while
refactoring the underlying implementation. A focused external-package test
checks what consumers can actually import without coupling the test to
unexported implementation details.

## Example

Run the external-package boundary check without connecting to PostgreSQL:

```sh
make postgres-migration-check
```

The same check is also included in `make check` and `make release-check`.

## Gotchas

- This check verifies the Go package surface, not PostgreSQL connectivity or
  SQL execution; the live integration test remains opt-in.
- The original parent-package API remains available during v1 development.
- Adding a public method or constant still requires updating the documented
  compatibility surface deliberately.

## Used in

- [`go/adapters/storage/postgres/postgres.go`](../../go/adapters/storage/postgres/postgres.go)
- [`go/adapters/storage/postgres/postgres_test.go`](../../go/adapters/storage/postgres/postgres_test.go)
- [`go/adapters/storage/postgres.go`](../../go/adapters/storage/postgres.go)
- [`go/examples/consumer/main.go`](../../go/examples/consumer/main.go)
- [`docs/notes/032-public-api-compatibility.md`](032-public-api-compatibility.md)

## Related

- [Vendor-specific storage should have an explicit optional import path](037-optional-postgres-package-boundary.md)
- [Storage schema versions should be separate from the Amber wire version](040-storage-schema-version-boundary.md)
- [The storage contract should be normative but separate from the core wire contract](041-normative-storage-contract.md)
