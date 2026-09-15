# Release identity should be decided separately from wire and storage versions

Amber has several kinds of version: the provenance wire contract, persistence
schema versions, the TypeScript package version, and the repository release
tag. They need explicit relationships without being treated as the same value.

## Origin

The TypeScript package is currently `0.1.0`, the Go module is nested inside the
InGen monorepo, and the repository has no Git tags. The release checklist
correctly deferred tag selection, but the facts and owner decisions had no
single location.

## What

The project now includes [`VERSIONING.md`](../../VERSIONING.md), which records:

- the current package, module, and repository state;
- the approved synchronized `0.1.0` semantic-version approach;
- the intended `amber/v0.1.0` monorepo tag form, pending repository release
  execution;
- semantic-versioning boundaries; and
- the version, package, tag, synchronized-release, and credential decisions
  required before publishing.

The release runbook links to this decision point, and no tag or package
publication was performed.

## Why

This prevents a release from accidentally conflating a package version with a
wire or storage contract version, and avoids inventing a monorepo tag convention
from the Amber subdirectory alone.

## Example

With repository-level release approval, use the `0.1.0` identity and intended
`amber/v0.1.0` tag from [`VERSIONING.md`](../../VERSIONING.md), then follow
[`RELEASE.md`](../../RELEASE.md) for local, hosted, database, and artifact
evidence.

## Gotchas

- The intended tag is approved as release metadata but has not been created.
- `0.1.0` is current TypeScript metadata, not proof that a public package has
  been published.
- Wire and storage versions must remain explicit even if the SDK release uses
  the same semantic version.
- The repository may contain unrelated workspace changes that must be reviewed
  before release preparation.

## Used in

- [`VERSIONING.md`](../../VERSIONING.md)
- [`RELEASE.md`](../../RELEASE.md)
- [`README.md`](../../README.md)
- [`typescript/package.json`](../../typescript/package.json)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [Release handoff should separate local proof from deployment-owned actions](058-release-handoff.md)
- [Contributor and release runbooks should make the verified workflow repeatable](060-contributor-release-runbooks.md)
- [The v1 foundation should have an explicit freeze boundary](028-v1-foundation-checkpoint.md)
