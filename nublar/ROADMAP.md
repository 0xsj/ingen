# Nublar roadmap

Roadmap date: 2026-09-17

Nublar is currently a local, filesystem-backed CI coordinator. It consumes
producer-written `ingen.ci-result/v1` envelopes, records immutable collection
runs, exposes history, and projects provider-neutral delivery decisions.

This roadmap separates completed work from possible future phases. A candidate
does not become implementation work until a concrete consumer establishes the
need and acceptance behavior.

## Current status

The first local slice is complete and frozen. Its verification command is:

```sh
make nublar-freeze-check
```

The freeze boundary is recorded in [`FREEZE-RECORD.md`](FREEZE-RECORD.md), and
the practical command flow is documented in
[`USAGE-GUIDE.md`](USAGE-GUIDE.md).

## Completed

### Collection and workflow policy

- Versioned workflow declarations with required and optional checks.
- Relative result-path validation, duplicate-path rejection, and producer/tool
  identity checks.
- Collection of already-produced `ingen.ci-result/v1` envelopes without
  launching producer workflows.
- Compatibility `aggregate` command for the earlier one-shot result shape.

### Strictness and provenance

- Closed workflow, envelope, run, decision, and receipt loading where required.
- Rejection of unknown fields and multiple JSON/YAML values.
- Exact SHA-256 references for the workflow and consumed result bytes.
- Persistence of failed and collection-error runs before returning their
  decision code.

### Run lifecycle and local storage

- Immutable `ingen.nublar-run/v1` records with opaque local `run_id` values.
- Optional provider-neutral external correlation (`system`, `id`, `attempt`).
- Atomic filesystem publication and deterministic newest-first history.
- Read-only `run show` and composable `run list` filters for status, workflow,
  and correlation.
- Explicit distinction between `run list` success and a selected run's
  decision code from `run show`.

### Decisions and delivery

- Provider-neutral `ingen.nublar-decision/v1` projection.
- Exclusion of producer-owned reports from the delivery projection.
- Generic HTTP webhook publisher with idempotency key and optional HMAC
  signature.
- Independent `ingen.nublar-delivery-receipt/v1` records.
- Optional immutable content-addressed local receipt history with filters.
- Repeated delivery preserves one run identity and creates separate receipts.

### Consumer proof and documentation

- Provider-neutral shell consumer example covering passed, failed, missing, and
  malformed collection paths plus history reads.
- End-to-end consumer coverage for projection, delivery, receipt persistence,
  and retry-shaped behavior.
- Usage guide, consumer handoff, contract checkpoint, freeze record, and
  consumer-request template.
- GitHub Actions fixture and live-handoff proofs with separate producer and
  Nublar consumer jobs, cross-job envelope transfer, and artifact inspection.
- Single `make nublar-freeze-check` gate covering tests, vet, schemas, consumer
  scenarios, and diff-whitespace validation.

## Frozen baseline

The following are stable for the current local alpha checkpoint:

- Schemas: workflow, run, decision, receipt, and the shared CI envelope.
- Commands: `workflow validate`, compatibility `aggregate`, `run collect`,
  `run show`, `run list`, `run decision`, `run deliver`, and `run receipt list`.
- Exit codes: `passed → 0`, `failed → 1`, `error → 2`.
- Run identity, correlation rules, artifact opacity, receipt independence, and
  filesystem publication semantics.

Changes to this baseline require a consumer brief, contract decision, an
executable regression, documentation updates, and a passing freeze gate. Use
[`CONSUMER-REQUEST-TEMPLATE.md`](CONSUMER-REQUEST-TEMPLATE.md) to start that
process.

## Candidate phases and decision gates

These are ordered by the evidence needed to begin them, not by a promise that
all of them will be built.

### 1. Concrete consumer integration — completed gate

Select one real CI or delivery consumer and document its required invocation,
outputs, authentication, retry behavior, scale, and acceptance cases. Use the
current provider-neutral shell example as the baseline and extend only the
surface the consumer demonstrably needs.

The first candidate is now a GitHub Actions CI job. Its intake and acceptance
boundary are documented in
[`GITHUB-ACTIONS-CONSUMER-BRIEF.md`](GITHUB-ACTIONS-CONSUMER-BRIEF.md). The
brief currently finds the frozen surface sufficient. The executable manual
workflow is [`../.github/workflows/nublar-consumer.yml`](../.github/workflows/nublar-consumer.yml);
it has now passed externally from `dev` at commit `b036117` in
[run 35186484244](https://github.com/0xsj/ingen/actions/runs/35186484244).
That proof covers the GitHub Actions invocation, correlation, artifact upload,
and decision propagation using a checked-in producer fixture. A live producer
handoff is now represented by a separate producer and Nublar consumer job in
the same workflow. The first live attempt identified the expected platform
boundary—Sorna requires its macOS host-enforcement backend—so the producer job
now runs on `macos-latest` while Nublar remains a separate POSIX consumer. The
corrected live handoff passed in
[run 35200478467](https://github.com/0xsj/ingen/actions/runs/35200478467),
including all four producer envelopes, cross-job transfer, and the final
passed/0 Nublar decision.

This gate is complete. The live consumer proof did not expose a missing Nublar
command, schema, or provider-specific delivery surface, so no implementation
work is opened from this phase.

Entry criteria:

- Completed consumer brief.
- Confirmed that the frozen commands and schemas are insufficient.
- Acceptance test that exercises the real consumer boundary.

### 2. Delivery adapter requirements

If the chosen consumer needs more than the generic webhook, define the
destination-specific mapping behind the decision projection. Possible work
includes authentication, key rotation, retry ownership, response handling, and
destination status semantics.

The first concrete destination is GitHub Checks. Its requirements draft is
[`GITHUB-CHECKS-ADAPTER-REQUIREMENTS.md`](GITHUB-CHECKS-ADAPTER-REQUIREMENTS.md).
The document defines the destination mapping, least-privilege authentication,
same-run idempotency, bounded retries, receipt behavior, and acceptance proof;
the adapter is implemented in `internal/delivery/githubchecks` and wired through
`run deliver --transport github-checks`. Local HTTP contract proof is complete;
live repository acceptance is also complete in
[run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692).
That run created GitHub Check Run `105146774540` with `completed/success` and
persisted an accepted `github-checks` receipt with HTTP status `201`.

This phase is complete. The next Nublar work should be driven by a new
destination requirement rather than expanding the GitHub adapter speculatively.

Entry criteria:

- Named destination and transport.
- Defined idempotency and retry behavior.
- Evidence that producer-owned reports remain opaque to the adapter.

### 3. Durable or hosted storage — evaluated and deferred

Only if the consumer needs multi-process, multi-machine, or long-lived history,
evaluate a hosted API, embedded database, remote store, or stronger filesystem
coordination. Retention, pagination, locking, migrations, and access control
belong to this phase.

The validated GitHub Actions consumer has now been evaluated in
[`STORAGE-EVALUATION.md`](STORAGE-EVALUATION.md). Its one-run-per-ephemeral-job
shape, 14-day artifact retention, small record sizes, and same-job query needs
do not meet this phase's entry criteria. No storage implementation work is
opened.

Entry criteria:

- Measured scale and retention requirement.
- Recovery and concurrency expectations.
- Defined query and authorization contract.

### 4. Producer execution and scheduling

If a consumer requires Nublar to launch producers, define orchestration
ownership, workspace isolation, cancellation, timeouts, scheduling, and
producer lifecycle reporting. The current boundary intentionally assumes that
producer workflows already ran.

Entry criteria:

- Explicit ownership transfer from the external workflow to Nublar.
- Required lifecycle and failure semantics.
- Isolation and cleanup contract.

### 5. Compatibility migration and SDKs

After a concrete consumer uses the run path, decide whether the compatibility
`aggregate` command should be deprecated or retained. Add SDKs or language
bindings only when a consumer has a stable need and the Go contract has proven
the required semantics.

Entry criteria:

- Consumer adoption evidence.
- Compatibility and migration plan.
- Contract tests independent of producer implementation details.

## Deliberately deferred

The following are not missing tasks in the current slice:

- Hosted Nublar server or remote API.
- Remote storage, retention, pagination, or retry queues.
- Provider-specific pull-request statuses, comments, or annotations.
- Nublar-launched producer execution or scheduling.
- Authentication policy and key rotation beyond the optional webhook HMAC
  mechanism.
- Reinterpretation of Sorna, Paddock, Sentinel, or other producer reports.

The next roadmap item should be selected by a concrete consumer, not by filling
this list speculatively.
