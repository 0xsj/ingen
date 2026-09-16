# Nublar

The proposed package and product layout is documented in
[`ARCHITECTURE.md`](ARCHITECTURE.md), and the ownership boundary is documented
in [`PRODUCT-BOUNDARY.md`](PRODUCT-BOUNDARY.md). The persisted run artifact
is described in [`RUN-ARTIFACT.md`](RUN-ARTIFACT.md), with run identity
defined in [`RUN-IDENTITY.md`](RUN-IDENTITY.md). Execution ownership is
defined in [`EXECUTION-BOUNDARY.md`](EXECUTION-BOUNDARY.md).
For the practical command flow, see [`USAGE-GUIDE.md`](USAGE-GUIDE.md).

Nublar is InGen's CI and delivery surface. Its first implementation slice is a
small coordinator that aggregates shared CI result envelopes. It may eventually
provide pull-request gates, scheduled mutation campaigns, hosted reports,
retention, and delivery-system integrations.

Nublar should consume Sorna's CLI, run specifications, evidence bundles, and
the shared [`ingen.ci-result/v1`](../core/CI-RESULT-SPEC.md) envelope produced
by tools such as Paddock. It should not reimplement architecture evaluation,
contract evaluation, or mutation semantics.

For Paddock, Nublar should store and surface the result status, exit code,
input hashes, deterministic report, and explanation. The nested Paddock report
remains the source of architectural findings; Nublar is a CI consumer, not a
second architecture engine.

## Workflow declaration

The workflow file declares which producer results a CI run expects:

```yaml
schema: ingen.nublar-workflow/v1
id: document-pipeline-ci
checks:
  - id: mutation-provider-review
    tool: sorna
    result: document-pipeline-go-provider-ci-result.json
    required: true
  - id: behavioral-verification
    tool: sorna
    result: document-pipeline-ci-result.json
    required: true
  - id: mutation-campaign
    tool: sorna
    result: document-pipeline-go-campaign-ci-result.json
    required: true
```

Checks are required by default. An explicitly false `required` value makes a
check optional. Paths are relative to the workspace root supplied to Nublar.

```sh
go run ./nublar/cmd/nublar workflow validate nublar/workflows/document-pipeline.yaml
go run ./nublar/cmd/nublar aggregate \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --output .artifacts/nublar-result.json
```

The workflow result paths are relative to the artifact root supplied with
`--root`; the repository Makefile supplies `.artifacts` by default and the
`nublar-aggregate-fresh` target supplies a new temporary root. Missing required results produce an `error` aggregate with exit code `2`.
Missing optional results are recorded as warnings. A result whose envelope
claims a different producer than the workflow declares is also an error. The
aggregate records the workflow path and SHA-256 so the collection policy is
part of the result provenance. Each consumed CI-result file is also recorded
with its own SHA-256.

The document-pipeline workflow collects Sorna's strict Go-provider preflight,
behavioral-verification, and strict Go mutation-campaign envelopes. Nublar
preserves all complete producer results and only composes their statuses;
Sorna remains the authority for contract, capability, plan-binding, and
mutation findings. The unbound prebuilt fixture remains available through the
local `mutation-campaign-*` targets but is not the default Nublar producer.

The webhook-validation workflow follows the same boundary with a separate
declaration: it supplies Sorna's webhook provider review, preparation,
behavioral-verification, and mutation-campaign envelopes without requiring
Nublar to understand webhook or mutation semantics.

## Run collection

The run-oriented command collects the workflow into an
`ingen.nublar-run/v1` artifact. It generates an opaque run ID when one is not
provided and records every declared check, including missing optional checks:

```sh
go run ./nublar/cmd/nublar run collect \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3 \
  --output .artifacts/nublar-run.json
```

The external correlation flags are optional and must be supplied together;
they add provider-neutral system, ID, and attempt metadata without changing
the run's immutable `run_id`.

The repository entry point is `make nublar-run-collect`; it also persists the
run under `NUBLAR_RUN_STORE` (default `.artifacts/nublar-runs`).
For a clean source/artifact proof, use `make nublar-run-collect-fresh`; it
creates a new workspace and run store while reusing the configured Go caches.

The existing `aggregate` command remains the compatibility command for the
older prototype; the run path is the first persisted collection record.

The first filesystem persistence boundary is documented in
[`STORAGE.md`](STORAGE.md).
The initial read contract is documented in
[`RUN-QUERY.md`](RUN-QUERY.md).
The CI-facing output and exit-code contract is documented in
[`OUTPUT-BOUNDARY.md`](OUTPUT-BOUNDARY.md).
The provider-neutral delivery projection is documented in
[`DELIVERY-BOUNDARY.md`](DELIVERY-BOUNDARY.md).
The optional local delivery-receipt store is documented in
[`RECEIPT-STORAGE.md`](RECEIPT-STORAGE.md).
The external consumer handoff is documented in
[`CONSUMER-GUIDE.md`](CONSUMER-GUIDE.md).
The current local contract checkpoint is documented in
[`CONTRACT-CHECKPOINT.md`](CONTRACT-CHECKPOINT.md).
The step-by-step local usage flow is documented in
[`USAGE-GUIDE.md`](USAGE-GUIDE.md).
The local freeze boundary and change rule are recorded in
[`FREEZE-RECORD.md`](FREEZE-RECORD.md).
The template for proposing the next concrete consumer requirement is
[`CONSUMER-REQUEST-TEMPLATE.md`](CONSUMER-REQUEST-TEMPLATE.md).
The provider-neutral CI gate example is documented in
[`examples/consumer/README.md`](examples/consumer/README.md).
The first Sentinel verifier handoff into Nublar is documented in
[`nublar-sentinel-verifier-workflow.md`](../notes/modules/nublar-sentinel-verifier-workflow.md).

The mixed-producer integration fixture demonstrates the intended neutrality
boundary: Nublar composes Sorna and Paddock envelopes while preserving each
producer report as opaque data. The consumer contract integration test carries
that run through collection, immutable storage, decision projection, and the
generic webhook publisher using an in-memory transport.

## Aggregate shared results

```sh
go run ./nublar/cmd/nublar aggregate \
  --output .artifacts/nublar-result.json \
  .artifacts/document-pipeline-ci-result.json \
  .artifacts/another-tool-ci-result.json
```

The command validates every `ingen.ci-result/v1` input, preserves the complete
producer artifact under `results`, and returns the highest envelope severity:
`error` (exit `2`) > `failed` (exit `1`) > `passed` (exit `0`). It does not read
Sorna's mutation fields or Paddock's architecture findings.

Nublar's collector uses the closed v1 envelope shape: unknown top-level fields
and multiple JSON values in one result file are rejected. Producer `report` and
`explanation` values remain opaque JSON and are preserved without interpretation.

The focused repository check is `make nublar-check`; it runs Nublar's race
tests and static analysis, then parses the Nublar and shared CI-envelope
schema files without running any producer workflow.

The provider-neutral consumer smoke check is `make nublar-consumer-check`; it
exercises the example CI gate against fixed passed, failed, missing-artifact,
and malformed-envelope fixtures and verifies the persisted runs, decision
projections, correlation metadata, expected exit codes, and read-only history
queries. The Nublar CLI delivery path is separately covered with accepted and
failed receipt-store regressions, while the end-to-end consumer test proves
repeated delivery keeps one run identity and separate receipt outcomes.
The history contract distinguishes successful listing from `run show`, which
returns the selected run's stored decision code.

Run `make nublar-freeze-check` to execute both the focused Nublar gate and the
provider-neutral consumer check, then validate diff whitespace, as one local
contract-freeze command.
