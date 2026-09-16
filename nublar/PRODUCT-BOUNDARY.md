# Nublar product boundary

## Purpose

Nublar is InGen's CI and delivery surface. It turns a declared collection of
producer checks into a reviewable run decision and makes the evidence needed
to explain that decision available to downstream CI and delivery systems.

The first product slice is local and filesystem-backed. It should establish
the run, workflow, artifact, and decision boundaries before Nublar gains hosted
execution, scheduling, or remote storage.

## First user workflow

Given a workflow declaration and an artifact root, Nublar should:

```text
load workflow
    → resolve declared checks
    → load and validate producer envelopes
    → record exact artifact and workflow hashes
    → create a run record
    → compose the coordinator decision
    → write the result for CI consumption
```

The first workflow is the existing document-pipeline workflow. Producer
commands remain outside Nublar for this slice: Sorna and Sentinel produce
shared result envelopes, and Nublar collects them. Sentinel's lifecycle receipt
is adapted to the shared envelope before it reaches Nublar.

The precise producer handoff and execution ownership are documented in
[`EXECUTION-BOUNDARY.md`](EXECUTION-BOUNDARY.md).

## Nublar owns

- Which checks a workflow expects and whether each check is required.
- Resolving declared result paths within an artifact root.
- Loading and validating shared `ingen.ci-result/v1` envelopes.
- Binding consumed files to their exact bytes with SHA-256 references.
- Detecting missing, duplicated, malformed, or incorrectly identified results.
- Composing the coordinator status and exit code.
- Preserving complete producer artifacts for review and downstream delivery.
- Representing the lifecycle and final state of a Nublar run.

Nublar's decision is based on the shared envelope status only:

```text
error > failed > passed
```

## Other systems own

| System | Owns | Nublar treats as |
| --- | --- | --- |
| Sorna | Contract evaluation, isolation, mutation semantics, evidence production | A producer of validated CI envelopes and opaque reports |
| Paddock | Architecture policy evaluation and findings | A producer of validated CI envelopes and opaque reports |
| Lockwood | Artifact custody, retention, lineage, and re-verification | A future storage or custody service |
| Sentinel | Agent workflow, permissions, workspace, lifecycle receipt, and interactive control | A producer of a shared CI envelope; Nublar treats the receipt report as opaque |
| Amber | Portable provenance and context semantics | A shared library or protocol dependency |

Nublar must not decide whether a mutation was killed, whether an architecture
rule is correct, or whether an observation gap is acceptable. Those are
producer-owned semantics carried inside the producer envelope and report.

## First-slice invariants

1. A workflow check is required unless it explicitly says `required: false`.
2. A declared result path is relative to the supplied artifact root.
3. A missing required result or producer identity mismatch is a coordinator
   error with exit code `2`.
4. A missing optional result is recorded as a warning.
5. Every consumed result is validated before it is included in the run.
6. Every consumed result and the workflow file are recorded with their exact
   SHA-256 hash.
7. Duplicate result paths are rejected.
8. The complete producer artifact is preserved; Nublar does not reconstruct its
   report or explanation.
9. An invalid input must not produce a partial, apparently successful run.
10. The result format is deterministic except for run metadata such as the
    creation timestamp.

## Explicit non-goals for the first slice

The first slice will not include:

- launching Sorna, Paddock, or other producers;
- provider-specific pull-request comments, status APIs, or annotations;
- scheduled or retried execution;
- a hosted Nublar server;
- artifact retention or a remote artifact store;
- approvals, branch rules, or organization policy;
- reinterpretation of producer reports or findings;
- a replacement for Sorna's evidence verification.

These may become later Nublar capabilities, but they should be added behind
the collection and run boundaries rather than embedded in the first aggregator.

## Initial command surface

The existing commands establish the first boundary:

```sh
nublar workflow validate <path>
nublar aggregate --workflow <path> --root <artifact-root> --output <path>
```

The implemented run-oriented command is:

```sh
nublar run collect --workflow <path> --root <artifact-root> --output <path> \
  [--external-system <name> --external-id <id> --attempt <n>]
nublar run show --store <dir> --run-id <id>
nublar run list --store <dir> [--status <passed|failed|error>] [--workflow <id>] \
  [--external-system <name>] [--external-id <id>] [--attempt <n>]
nublar run decision --store <dir> --run-id <id>
nublar run deliver --store <dir> --run-id <id> --webhook <url> [--receipt <path>] [--receipt-store <dir>]
nublar run receipt list --receipt-store <dir> [--run-id <id>] [--status <accepted|failed>] [--transport <name>] [--output <path>]
```

It emits and can persist the `ingen.nublar-run/v1` record. `aggregate` remains
available as the compatibility command for the older
`ingen.nublar-result/v1` prototype.

## Decisions still intentionally open

- Whether the first durable run store should remain filesystem-only or use a
  small embedded database.
- Whether the first external integration needs explicit rerun relationships
  beyond the current per-attempt `run_id` and optional correlation block in
  [`RUN-IDENTITY.md`](RUN-IDENTITY.md).
- Whether workflow declarations eventually include producer commands or only
  describe expected artifacts.
- Which hosted provider, authentication model, and retry store should sit
  behind the generic webhook transport.
- How the proposed `ingen.nublar-run/v1` record should transition from the
  current `ingen.nublar-result/v1` prototype output.

These questions should be answered by the first end-to-end consumer, not by
adding speculative infrastructure now.
