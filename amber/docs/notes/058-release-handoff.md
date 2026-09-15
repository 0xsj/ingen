# Release handoff should separate local proof from deployment-owned actions

A release document should make it obvious which claims Amber has proved locally
and which actions still require access to a repository, registry, or deployment.

## Origin

Amber's local release-candidate gate and disposable PostgreSQL verification now
pass, but the root README still described the completed implementation as
planned. Hosted GitHub Actions status and npm publishing also cannot be
verified without credentials.

## What

The root README now identifies the implemented capabilities and summarizes the
release handoff:

- `make release-check` is the local release-candidate gate;
- PostgreSQL deployment requires applying the checked-in migration and then
  running readiness and live integration checks;
- hosted CI results and npm publishing remain credentialed release actions.

The detailed evidence and deployment-owner decisions remain in
[`docs/release-readiness.md`](../release-readiness.md).

## Why

This keeps local evidence honest while still making the next operational actions
easy to find. It also avoids treating a successful local PostgreSQL 16 run as
proof of the managed database configuration or treating an unpublished package
as a released artifact.

## Example

The local evidence command is:

```sh
make release-check
```

The deployment-owner sequence is:

```sh
# Apply 001_amber_provenance.sql through application migration tooling.
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-schema-check
AMBER_POSTGRES_DSN='postgres://user:password@host:5432/amber?sslmode=disable' \
  make postgres-integration
```

## Gotchas

- GitHub Actions results require authenticated repository access; no hosted
  result is claimed from this workspace.
- npm publishing is intentionally not performed as part of local verification.
- The migration is application-owned; `postgres-schema-check` does not create
  or migrate tables.
- Existing unrelated workspace changes must be reviewed separately before a
  release commit is prepared.

## Used in

- [`README.md`](../../README.md)
- [`docs/release-readiness.md`](../release-readiness.md)
- [`CHANGELOG.md`](../../CHANGELOG.md)
- [`Makefile`](../../Makefile)

## Related

- [Local PostgreSQL verification should follow the migration-before-readiness sequence](057-local-postgres-verification.md)
- [Release readiness should have one repeatable project gate](033-release-candidate-gate.md)
- [Amber needs a release-readiness matrix before more generic expansion](046-release-readiness-matrix.md)
