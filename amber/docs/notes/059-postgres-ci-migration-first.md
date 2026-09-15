# PostgreSQL CI should exercise the checked-in migration before adapter writes

An integration test that bootstraps its own schema can hide a broken migration
asset. CI should validate the migration path that an application will actually
use before it exercises persistence.

## Origin

The PostgreSQL service job ran `make postgres-integration`, whose test called
`EnsureSchema`, and then ran the read-only schema check. That proved the adapter
and schema declaration, but the test could create the schema without executing
the checked-in SQL migration.

## What

The PostgreSQL workflow now:

1. applies `001_amber_provenance.sql` with `make postgres-apply-migration`;
2. runs `make postgres-schema-check` as a read-only readiness probe; and
3. runs `make postgres-integration` against the existing schema.

The live integration test now calls `CheckSchema` instead of `EnsureSchema`, so
it requires the migration to be present and does not create tables or indexes
as a side effect. The new Make target is a convenience for local `psql`
environments; production applications still own migration tooling.

## Why

This closes the gap between migration verification and adapter verification. A
changed or incomplete migration now fails before the live contract can repair
the database through bootstrap DDL. The default `make release-check` remains
offline and unchanged.

## Example

```sh
AMBER_POSTGRES_DSN='postgres://user:password@localhost:5432/amber?sslmode=disable' \
  make postgres-apply-migration
AMBER_POSTGRES_DSN='postgres://user:password@localhost:5432/amber?sslmode=disable' \
  make postgres-schema-check
AMBER_POSTGRES_DSN='postgres://user:password@localhost:5432/amber?sslmode=disable' \
  make postgres-integration
```

## Gotchas

- `postgres-apply-migration` requires `psql` and a DSN; it is not part of the
  offline release gate.
- `postgres-schema-check` is intentionally read-only and does not apply the
  migration.
- `EnsureSchema` remains available for simple application-owned bootstrap
  flows; the live CI contract deliberately tests the migration-first path.
- The workflow still does not verify a production PostgreSQL version,
  credentials, pooling, backup, or recovery policy.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`Makefile`](../../Makefile)
- [`go/adapters/storage/postgres_integration_test.go`](../../go/adapters/storage/postgres_integration_test.go)
- [`go/adapters/storage/postgres/migrations/001_amber_provenance.sql`](../../go/adapters/storage/postgres/migrations/001_amber_provenance.sql)

## Related

- [PostgreSQL compatibility should be testable against a real database without entering the default gate](035-postgres-live-integration.md)
- [CI should verify PostgreSQL read-only schema readiness](048-postgres-ci-schema-readiness-check.md)
- [Local PostgreSQL verification should follow the migration-before-readiness sequence](057-local-postgres-verification.md)
