# Local PostgreSQL verification should follow the migration-before-readiness sequence

Environment-dependent adapter checks are most useful when they prove the same
order an application will use in deployment: provision a database, apply the
application-owned migration, validate readiness, and then exercise persistence.

## Origin

Amber had offline PostgreSQL tests, an opt-in live integration target, a
read-only schema check, and a CI service job. The local release-readiness matrix
still had no evidence from a real PostgreSQL server.

## What

On 2026-09-15, a disposable PostgreSQL 16 Docker container was started locally
for Amber verification. The following checks were run with
`AMBER_POSTGRES_DSN` pointing at that container:

- `make postgres-schema-check` was first run against the empty database and
  correctly failed because the readiness metadata table did not exist.
- The checked-in `001_amber_provenance.sql` migration was applied.
- `make postgres-schema-check` then passed and reported `amber_provenance v1`.
- `make postgres-integration` was rerun against that migrated schema and passed
  the live `PostgresStore` contract test.

The temporary container was removed after the checks. The PostgreSQL image was
not treated as project state, and no production or managed-database claim is
made by this local verification.

## Why

This proves that the adapter can persist against a real PostgreSQL server and
that readiness remains a read-only post-migration check. Keeping the initial
fresh-database failure visible also documents an important operational rule:
`postgres-apply-migration` or equivalent application tooling must run before
`postgres-schema-check` and `postgres-integration`; neither Amber check creates
or migrates tables for the operator.

## Example

The deployment sequence is:

```sh
# Apply through the application's migration tooling, or locally with:
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-apply-migration

AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-schema-check

AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-integration
```

For local disposable verification, PostgreSQL 16 can be substituted for the
deployment host while retaining the same Amber commands.

## Gotchas

- The schema readiness command is intentionally read-only; a fresh database is
  expected to fail until the migration has been applied.
- The live integration test uses a transaction that is rolled back, so it
  exercises persistence without leaving test provenance rows behind.
- A local PostgreSQL 16 pass does not verify managed PostgreSQL configuration,
  credentials, pooling, backups, retention, or recovery policy.
- The default `make release-check` remains offline and does not require a
  database service.

## Used in

- [`go/adapters/storage/postgres_integration_test.go`](../../go/adapters/storage/postgres_integration_test.go)
- [`go/examples/postgres/main.go`](../../go/examples/postgres/main.go)
- [`go/adapters/storage/postgres/migrations/001_amber_provenance.sql`](../../go/adapters/storage/postgres/migrations/001_amber_provenance.sql)
- [`Makefile`](../../Makefile)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [PostgreSQL compatibility should be testable against a real database without entering the default gate](035-postgres-live-integration.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
- [CI should run the live PostgreSQL contract without changing the offline gate](045-postgres-ci-integration-job.md)
- [PostgreSQL should ship a checked-in migration asset with readiness guidance](054-postgres-migration-asset.md)
- [PostgreSQL migration readiness should have an offline preflight](055-postgres-migration-preflight.md)
