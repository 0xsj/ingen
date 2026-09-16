# Nublar local contract freeze record

Freeze date: 2026-09-16

This records the current local Nublar alpha boundary. It is a filesystem-backed
coordinator and provider-neutral consumer surface; it is not a hosted Nublar
implementation.

## Frozen surface

- `ingen.nublar-workflow/v1` declares required and optional producer result
  paths.
- `ingen.ci-result/v1` is collected with strict closed-envelope loading and
  byte-level provenance.
- `ingen.nublar-run/v1` records one immutable collection attempt, including
  status, exit code, preserved producer artifacts, and optional external
  correlation.
- `ingen.nublar-decision/v1` is the compact provider-neutral delivery
  projection and excludes producer-owned reports.
- `ingen.nublar-delivery-receipt/v1` records independent delivery attempts;
  receipt history is optional, immutable, content-addressed local storage.
- `run show`, `run list`, `run decision`, `run deliver`, and `run receipt list`
  are the current CLI read, projection, delivery, and receipt surfaces.
- The provider-neutral consumer example covers passed, failed, missing,
  malformed, and history paths.
- The end-to-end consumer test covers delivery, receipt persistence, and
  retry-shaped delivery while preserving one run identity.

## Freeze proof

From the repository root:

```sh
make nublar-freeze-check
```

The freeze gate runs Nublar's race tests, static analysis, schema checks, the
consumer contract check, strict-ingestion scenarios, history queries, and
decision/receipt assertions, then checks diff whitespace.

## Change rule

Any future change to this surface should identify the concrete consumer need,
update the relevant schema and documentation, add an executable regression,
and rerun `make nublar-freeze-check`.
The request shape is available in
[`CONSUMER-REQUEST-TEMPLATE.md`](CONSUMER-REQUEST-TEMPLATE.md).

## Deferred

Hosted APIs, remote storage, retention and pagination, delivery retry policy,
authentication and key rotation, producer execution, scheduling, and
provider-specific delivery formats remain outside this freeze.
