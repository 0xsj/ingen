# Contributing to Amber

Amber is a contract-first provenance library with Go and TypeScript SDKs. The
language-neutral specifications under `spec/` define required behavior; SDKs
and adapters should remain aligned with those contracts.

## Prerequisites

- Go matching the version declared in [`go/go.mod`](go/go.mod).
- Node.js 22 and npm for the TypeScript SDK.
- Docker and `psql` only when running the optional local PostgreSQL checks.

Install TypeScript dependencies before running the first TypeScript check:

```sh
cd typescript
npm ci
cd ..
```

## Development checks

From the Amber project root:

```sh
make test          # Go, TypeScript, and shared conformance tests
make check         # static checks, tests, package, module, and migration checks
make release-check # check, race, fuzz, and runnable examples
```

Use `make examples` when changing an adapter composition or reference flow.
Use `make race` when changing concurrent storage behavior. Keep the default
checks offline and deterministic; live services belong in opt-in targets or
CI service jobs.

## Optional PostgreSQL verification

The PostgreSQL adapter is optional. Applications own the driver, pool,
migration lifecycle, and transaction boundaries. Against a disposable or
isolated database, apply the checked-in migration, verify readiness, and then
run the live contract:

```sh
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-apply-migration
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-schema-check
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-integration
```

`postgres-schema-check` is read-only. `postgres-integration` requires an
already-migrated schema and rolls back its provenance writes. The offline
`make postgres-migration-check` target only checks the migration asset boundary
and does not connect to PostgreSQL.

## Change boundaries

- Update the relevant specification before changing required wire or storage
  behavior.
- Preserve immutable transitions, append-only storage semantics, and explicit
  incoming failure policy.
- Keep framework, broker, database, logging, tracing, and OpenTelemetry
  dependencies in adapters rather than the core model.
- Add or update shared conformance fixtures when behavior is observable across
  Go and TypeScript.
- Update the applicable implementation note and the milestone index when a
  design or verification result changes.

## Before opening a change

Run the narrowest relevant checks during iteration, then run:

```sh
make release-check
git diff --check
```

Do not include unrelated workspace changes in an Amber change. The release
procedure is documented in [`RELEASE.md`](RELEASE.md).
