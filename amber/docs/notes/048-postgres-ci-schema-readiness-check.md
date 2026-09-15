# CI should verify PostgreSQL read-only schema readiness

The live PostgreSQL job should cover both adapter behavior and the readiness
path operators use after application-managed migrations.

## Origin

The PostgreSQL CI job already created the schema and ran the shared storage
contract, but only the opt-in operator command exercised `CheckSchema` through
the standalone example. That left the read-only command dependent on local or
deployment-specific verification.

## What

The PostgreSQL GitHub Actions job now runs `make postgres-schema-check` after
`make postgres-integration`. The integration test leaves the idempotently
created schema in the disposable database while rolling back its provenance
writes, so the readiness command can inspect the migrated metadata without
adding application data.

The normal `make release-check` remains service-free and does not run either
PostgreSQL command.

## Why

This keeps the two operational contracts explicit:

- the live integration check proves writes, reads, ordering, JSONB decoding,
  and transaction interaction against PostgreSQL;
- the read-only command proves that an already-migrated database reports the
  schema version expected by the adapter.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`Makefile`](../../Makefile)
- [`go/examples/postgres/main.go`](../../go/examples/postgres/main.go)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [CI should run the live PostgreSQL contract without changing the offline gate](045-postgres-ci-integration-job.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
