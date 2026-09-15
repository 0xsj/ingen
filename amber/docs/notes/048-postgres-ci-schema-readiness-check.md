# CI should verify PostgreSQL read-only schema readiness

The live PostgreSQL job should cover both adapter behavior and the readiness
path operators use after application-managed migrations.

## Origin

The PostgreSQL CI job already created the schema and ran the shared storage
contract, but only the opt-in operator command exercised `CheckSchema` through
the standalone example. That left the read-only command dependent on local or
deployment-specific verification.

## What

The PostgreSQL GitHub Actions job now applies the checked-in migration with
`make postgres-apply-migration`, runs `make postgres-schema-check` before the
live contract, and then runs `make postgres-integration`. The live test checks
the existing schema and rolls back its provenance writes, so CI exercises the
same migration-before-readiness-before-writes sequence used by deployment.

The normal `make release-check` remains service-free and does not run either
PostgreSQL command.

## Why

This keeps the two operational contracts explicit:

- the live integration check proves writes, reads, ordering, JSONB decoding,
  and transaction interaction against PostgreSQL;
- the read-only command proves that an already-migrated database reports the
  schema version expected by the adapter.

## Example

The PostgreSQL workflow's migration-first sequence is:

```sh
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-apply-migration
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-schema-check
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-integration
```

## Gotchas

- The service job uses PostgreSQL 16; managed deployments still require their
  own compatibility check.
- Hosted workflow results are external evidence and must not be inferred from
  a local `make release-check` pass.
- The default release gate remains service-free and does not run these targets.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`Makefile`](../../Makefile)
- [`go/examples/postgres/main.go`](../../go/examples/postgres/main.go)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [CI should run the live PostgreSQL contract without changing the offline gate](045-postgres-ci-integration-job.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
