# Amber v1 release-readiness matrix

Last local verification: 2026-09-15.

## Ready and locally verified

| Area | Evidence | Status |
| --- | --- | --- |
| Core provenance model | `spec/v1.md`, Go and TypeScript transition tests, shared v1 fixtures | Ready |
| Context and incoming boundaries | Scoped restoration, reject/ignore policy, byte limits, trust-validator hooks | Ready |
| HTTP and messaging adapters | Round-trip fixtures, middleware tests, malformed-input coverage | Ready |
| Logging, tracing, and OpenTelemetry | Stable `amber.*` projections, SDK-backed tests, runnable examples | Ready |
| Go storage | Memory, file, generic key-value, reusable contract suite, race coverage | Ready |
| TypeScript storage | Memory and generic key-value stores, contract and concurrent Promise coverage | Ready |
| PostgreSQL offline behavior | SQL mock tests, schema metadata/version checks, explicit optional package path | Ready |
| PostgreSQL migration asset | Checked-in v1 SQL migration is embedded by the optional package and tested against `PostgresSchema` | Ready for application-owned migration tooling |
| Package/module boundaries | TypeScript tarball smoke test, public API manifest, external Go consumer smoke test | Ready |
| Verification | `make release-check` passes locally, including vet, typecheck, tests, race, fuzz, and examples | Ready |
| Reference deployment vertical | Runnable Go `net/http` service composes inbound propagation, child derivation, injected storage, and response propagation | Ready as reference flow |

## Environment-dependent verification

| Check | Command | Current status |
| --- | --- | --- |
| PostgreSQL full integration | `AMBER_POSTGRES_DSN=... make postgres-integration` | Passed locally against disposable PostgreSQL 16 |
| PostgreSQL read-only readiness | `AMBER_POSTGRES_DSN=... make postgres-schema-check` | Passed locally against disposable PostgreSQL 16 after applying the v1 migration |
| Managed PostgreSQL compatibility | Run the live check against the deployment's PostgreSQL version | Deployment owner must verify |
| GitHub Actions service job | `.github/workflows/ci.yml` `postgres` job | Configured for live integration plus read-only schema readiness; hosted-run result is external to this workspace |

## Deployment-owner decisions

- Select the application-owned `KeyValueBackend`, or adopt the optional
  `amberpostgres` package.
- Own database credentials, connection pooling, migrations, transaction scope,
  retention, backups, and recovery policy.
- Apply `001_amber_provenance.sql` through the deployment's migration tooling,
  then run the read-only schema check before enabling writes.
- Run the live PostgreSQL checks against an isolated or disposable database and
  the managed PostgreSQL version used in production.
- Decide npm publishing ownership and whether `@0xsj/amber` remains the final
  package name.
- Select any broker-specific or transport-specific adapter required by the
  deployment.

## Explicitly out of scope for Amber v1

- Authorization, authentication, or claims that provenance proves correctness.
- Signed envelopes, key rotation, replay protection, and asynchronous trust
  infrastructure.
- A complete ancestry graph, causal topological traversal, query language,
  pagination, retention, or recovery guarantees.
- Cross-process locking for the Go file store.
- Durability, transactions, replication, or atomicity guarantees for a
  user-provided generic backend beyond its documented contract.
- Production collector configuration or a mandatory OpenTelemetry runtime.

## Recommended stopping point

Amber should not gain new core fields, enum values, or transitions without a
concrete consumer requirement and a reviewed protocol extension. The next
implementation cycle should choose one deployment vertical, run its
environment-dependent checks, and add only the adapter or operational policy
that vertical requires.

The first reference vertical is Go `net/http` with an injected `Store`; it does
not make Go, `net/http`, or the file store mandatory for other deployments.

The repeatable local checkpoint is:

```sh
make release-check
```

The hosted and deployment-specific checkpoint is:

```sh
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-integration
```

The local live PostgreSQL checkpoint completed on 2026-09-15 against a
disposable PostgreSQL 16 container. The integration check passed, the checked-in
`001_amber_provenance.sql` migration was applied, and the read-only schema check
reported `amber_provenance v1`. This verifies the local migration and adapter
sequence; the managed PostgreSQL version remains a deployment-owner check.
