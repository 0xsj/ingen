# Deferred deployment should be separated from consumer-driven development

Amber can continue improving locally while release ownership and deployment
operations remain intentionally deferred.

## Origin

The local v1 release candidate is fully verified, but npm authentication,
hosted CI inspection, managed PostgreSQL compatibility, release tagging, and
publishing require repository or deployment ownership. Repeating those checks
did not create new implementation value, while future features still need a
concrete consumer scenario.

## What

The repository now has [`BACKLOG.md`](../../BACKLOG.md), which separates:

- deferred npm, GitHub, PostgreSQL, tag, and publication actions; and
- future development intake requiring a runtime, boundary, observable behavior,
  ownership boundary, and verification plan.

The backlog points to the release-readiness and versioning documents and keeps
v1 non-goals visible. It does not change code, contracts, or release state.

## Why

This gives the project a clear resumption point. Deployment can wait without
being mistaken for unfinished core implementation, and new adapters or protocol
extensions must be justified by an actual consumer rather than momentum.

## Example

A future feature should enter with a concrete statement such as: a named
runtime or boundary needs a specific Amber value or adapter behavior that is
currently unavailable, plus a testable acceptance condition.

Release operations remain separate and can be resumed from:

```sh
make release-check
```

followed by the owner-controlled steps in [`RELEASE.md`](../../RELEASE.md).

## Gotchas

- A passing local release gate does not prove hosted CI, npm ownership, or
  managed-database compatibility.
- A backlog candidate is not an approved protocol change.
- Signed transport, retention, recovery, pagination, and advanced ancestry
  remain v1 non-goals until a reviewed extension is required.
- No tag, commit, push, or publication is created by this backlog.

## Used in

- [`BACKLOG.md`](../../BACKLOG.md)
- [`README.md`](../../README.md)
- [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
- [`RELEASE.md`](../../RELEASE.md)
- [`VERSIONING.md`](../../VERSIONING.md)
- [`docs/release-readiness.md`](../release-readiness.md)

## Related

- [Release handoff should separate local proof from deployment-owned actions](058-release-handoff.md)
- [Amber needs a release-readiness matrix before more generic expansion](046-release-readiness-matrix.md)
- [Release identity should be decided separately from wire and storage versions](063-versioning-decision.md)
- [Custom storage adapters should have reusable contract-test helpers](068-reusable-storage-contract-helpers.md)
