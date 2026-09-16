# Amber backlog

This backlog separates deferred release operations from future implementation
work. It is intentionally conservative: Amber's v1 core and existing adapter
boundaries should not expand without a concrete consumer requirement.

## Current checkpoint

- The v1 wire and storage contracts are implemented in Go and TypeScript.
- The local `make release-check` gate passes.
- HTTP, messaging, worker, storage, logging, tracing, and optional
  OpenTelemetry reference flows are covered by runnable examples.
- Custom storage authors have generic seams and reusable contract-test helpers.

## Deferred release operations

These are valid next steps for a release owner, but they are not required for
continued local development:

- authenticate npm and confirm ownership of `@0xsj/amber`;
- confirm the final npm scope/package name;
- inspect hosted GitHub Actions results;
- run PostgreSQL checks against the managed deployment version;
- prepare the approved `0.1.0` release commit and changelog section; and
- create the repository-qualified `amber/v0.1.0` tag and publish only after
  explicit approval.

See [`RELEASE.md`](RELEASE.md), [`VERSIONING.md`](VERSIONING.md), and
[`docs/release-readiness.md`](docs/release-readiness.md) for the evidence and
stop conditions.

## Future development intake

New implementation work should start with a concrete consumer scenario that
answers:

1. What runtime or boundary is involved?
2. Which existing Amber value, transition, adapter, or storage behavior is
   insufficient?
3. What behavior must be observable across Go and TypeScript, if any?
4. Which application-owned policies remain outside Amber?
5. How will the change be verified without making unrelated deployments
   mandatory?

Potential work may include a broker-specific adapter, another runtime vertical,
or a reviewed authenticated transport extension. These remain candidates, not
commitments. Pagination, retention, recovery, signed envelopes, and advanced
ancestry queries remain explicit v1 non-goals until a reviewed extension is
required.

## Resumption rule

When work resumes, select one backlog item, write or update its implementation
note, define its narrow verification command, and stop at the corresponding
checkpoint. Do not infer release ownership or deployment policy from a local
test result.
