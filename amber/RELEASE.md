# Amber release checklist

This checklist separates evidence that can be produced locally from actions
that require repository, registry, or deployment access. It is a handoff guide,
not an instruction to publish automatically.

## 1. Review scope and compatibility

- Confirm the intended version and package name with the release owner.
- Review [`VERSIONING.md`](VERSIONING.md); the prepared identity is `0.1.0`
  with intended tag `amber/v0.1.0`, pending repository-level release approval.
- Review changes to [`spec/`](spec/), public Go packages, and the TypeScript
  export manifest for compatibility impact.
- Confirm the changelog describes user-visible behavior and any wire or storage
  changes.
- Review unrelated dirty workspace changes before preparing a release commit.

## 2. Run local evidence

From the Amber project root:

```sh
make release-check
git diff --check
```

This covers Go vet/tests, TypeScript typechecking/tests, shared conformance,
package and module consumer checks, migration preflight, race detection, fuzz
targets, and runnable examples.

The TypeScript test suite also verifies that `VERSION`, `package.json`, and
`package-lock.json` agree on `0.1.0`.

## 3. Verify PostgreSQL when it is in scope

Against an isolated database running the intended PostgreSQL version:

1. Apply [`001_amber_provenance.sql`](go/adapters/storage/postgres/migrations/001_amber_provenance.sql)
   through application-owned migration tooling.
2. Run `make postgres-schema-check` with `AMBER_POSTGRES_DSN` set.
3. Run `make postgres-integration` with the same DSN.
4. Record the database version and result in
   [`docs/release-readiness.md`](docs/release-readiness.md).

The migration must precede readiness and live integration. Amber does not own
production credentials, pooling, backups, retention, recovery, or migration
history.

## 4. Confirm hosted verification

The GitHub Actions workflow should pass both jobs:

- `verify`: static checks, tests, package/module checks, race checks, and
  examples;
- `postgres`: checked-in migration, read-only readiness, and live adapter
  integration against PostgreSQL 16.

Hosted results require authenticated access to the repository. Do not claim a
hosted result based only on local execution.

## 5. Inspect artifacts and publish deliberately

- Confirm `make package-check` passes and inspect the tarball contents.
- Confirm the package contains its README, MIT license, metadata, declarations,
  and runtime entry point.
- Confirm the Go module consumer check passes from outside the module.
- Publish only after the release owner confirms registry ownership, package
  scope, version, and authentication.
- Tag or release the monorepo according to its repository-level policy; do not
  invent a tag convention from the Amber subdirectory.

## Stop conditions

Stop and resolve the issue before publishing if any of these apply:

- `make release-check` fails;
- migration preflight or the managed PostgreSQL check fails;
- hosted CI is missing or reports a failure;
- the package name, version, repository metadata, or license is uncertain;
- a public wire/storage contract changed without a reviewed specification update.

The detailed status matrix is [`docs/release-readiness.md`](docs/release-readiness.md).
