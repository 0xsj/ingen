# Contributor and release runbooks should make the verified workflow repeatable

Once the implementation reaches a release-readiness checkpoint, the next
failure mode is operational ambiguity: a contributor or release owner may not
know which checks are required, which services are optional, or which actions
need credentials.

## Origin

Amber had a detailed readiness matrix and implementation notes, but no short
entry point for contributors or release owners. The local release gate,
migration-first PostgreSQL flow, and package/module checks were individually
documented across several files.

## What

The project now includes:

- [`CONTRIBUTING.md`](../../CONTRIBUTING.md) for prerequisites, development
  checks, optional PostgreSQL verification, and change boundaries;
- [`RELEASE.md`](../../RELEASE.md) for compatibility review, local evidence,
  managed-database verification, hosted CI, artifact inspection, publishing,
  and stop conditions.

The root README links both documents, and the notes index records them as a
release-readiness milestone.

## Why

This makes the proven workflow repeatable without turning optional PostgreSQL
or credentialed publishing actions into mandatory local setup. It also gives
future contributors a clear place to start before changing the core contract
or an adapter boundary.

## Example

The normal local path remains:

```sh
make release-check
git diff --check
```

The PostgreSQL path remains opt-in and migration-first; the release runbook
points to the checked-in migration, readiness probe, and live contract target.

## Gotchas

- Local green checks do not prove hosted CI, registry ownership, or managed
  PostgreSQL compatibility.
- The release runbook intentionally does not publish packages or create tags.
- The root `ingen` workspace may contain unrelated changes; those require a
  separate review before release preparation.
- New required wire or storage behavior still needs a specification update and
  cross-language verification.

## Used in

- [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
- [`RELEASE.md`](../../RELEASE.md)
- [`README.md`](../../README.md)
- [`docs/release-readiness.md`](../release-readiness.md)
- [`docs/notes/README.md`](README.md)

## Related

- [Release handoff should separate local proof from deployment-owned actions](058-release-handoff.md)
- [PostgreSQL CI should exercise the checked-in migration before adapter writes](059-postgres-ci-migration-first.md)
- [Amber needs a release-readiness matrix before more generic expansion](046-release-readiness-matrix.md)
