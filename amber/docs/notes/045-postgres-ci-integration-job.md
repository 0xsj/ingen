# CI should run the live PostgreSQL contract without changing the offline gate

The PostgreSQL adapter needs real-database coverage, but local and default
release checks should remain deterministic and service-free.

## Origin

Amber had an opt-in `make postgres-integration` target and a read-only schema
check, but no hosted workflow provisioned PostgreSQL automatically. A pull
request could therefore pass all default checks while never exercising JSONB,
the SQL dialect, schema metadata, indexes, or transaction interaction against
the real database engine.

## What

The GitHub Actions workflow now has a separate `postgres` job with a PostgreSQL
16 service container. It applies the checked-in migration, runs
`make postgres-schema-check`, and then runs `make postgres-integration` against
the existing schema. The live contract executes storage operations inside a
transaction that is rolled back at test completion.

The existing `verify` job and local `make release-check` remain unchanged and
offline. The PostgreSQL job runs in parallel and has no repository write or
secret permissions.

## Why

This gives every pull request real SQL compatibility evidence while preserving
the fast, reproducible local workflow. Keeping the job separate also makes a
database failure visible as an adapter-specific CI failure rather than
obscuring the language-level checks.

## Example

The workflow's migration-first sequence is reproducible locally with:

```sh
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-apply-migration
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-schema-check
AMBER_POSTGRES_DSN='postgres://amber:amber@localhost:5432/amber?sslmode=disable' \
  make postgres-integration
```

## Gotchas

- The service version is pinned to PostgreSQL 16; deployments should still run
  the live check against their managed PostgreSQL version before rollout.
- The test database is disposable and uses a fixed CI-only credential; no
  production DSN or secret is required.
- GitHub-hosted runner and container availability remain external CI
  dependencies.
- The schema-check command runs before integration so the job verifies the
  operator-facing readiness path before adapter writes.

## Used in

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- [`Makefile`](../../Makefile)
- [`go/adapters/storage/postgres_integration_test.go`](../../go/adapters/storage/postgres_integration_test.go)
- [`docs/notes/035-postgres-live-integration.md`](035-postgres-live-integration.md)
- [`docs/notes/044-postgres-schema-check-example.md`](044-postgres-schema-check-example.md)

## Related

- [CI should run the same cross-language checks and examples as local development](016-continuous-verification-workflow.md)
- [PostgreSQL compatibility should be testable against a real database without entering the default gate](035-postgres-live-integration.md)
- [The PostgreSQL schema readiness check should be runnable without writes](044-postgres-schema-check-example.md)
