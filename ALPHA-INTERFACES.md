# InGen alpha interfaces

This is the current Sorna/Nublar/Sentinel alpha boundary. It names the artifacts and
semantics that another tool may consume today; it does not promise that the
alpha is a long-term compatibility commitment.

## What this checkpoint freezes

For the current document-pipeline slice, a producer must keep the following
schema identities and meanings stable until an intentional interface review:

| Boundary | Schema | Owner | Current meaning |
| --- | --- | --- | --- |
| Contract source/sealed artifact | `ingen.contract/v1` | Sorna | Reviewed behavioral intent that can be canonicalized and sealed. |
| Capability policy | `ingen.policy/v1` | Sorna | Declared capabilities and enforcement requirements for a run. |
| Frozen oracle | `ingen.oracle/v1` | Sorna | Materialized cases and expectations produced before subject execution. |
| Mutation catalogue | `ingen.mutation-catalogue/v1` | Sorna | Reviewable mutation declarations before a campaign plan exists. |
| Campaign plan | `ingen.mutation-plan/v1` | Sorna | Hash-bound handoff from contract/oracle/baseline review to a provider. |
| Mutation provider | `ingen.mutation-provider/v1` | Provider | Prepared variants, capabilities, provenance, and executable identities. |
| Provider review | `ingen.mutation-provider-review/v1` | Sorna | No-execution capability and plan-binding decision. |
| Preparation summary | `ingen.mutation-preparation/v1` | Provider | Changed files, target resolution, hashes, and prepared variants before execution. |
| Campaign result | `ingen.mutation-campaign-result/v1` | Sorna | One clean comparison and one outcome for each planned mutation. |
| CI result envelope | `ingen.ci-result/v1` | InGen core | Language-neutral status, exit code, source identity, input hashes, and opaque producer report. |
| Replay matrix report | `sorna.replay-matrix/v1` | Sorna | Expected-versus-observed classifications for a set of behavioral replay CI envelopes. |
| Replay matrix explanation | `sorna.replay-matrix-explanation/v1` | Sorna | Compact mismatch summary paired with the replay matrix report. |
| Replay matrix manifest | `sorna.replay-matrix-manifest/v1` | Sorna | Reviewable paths and expected classifications used to produce a replay matrix. |
| Nublar workflow | `ingen.nublar-workflow/v1` | Nublar | Required/optional CI result paths resolved under an artifact root. |
| Nublar aggregate | `ingen.nublar-result/v1` | Nublar | Preserved input envelopes plus severity composition. |
| Nublar run | `ingen.nublar-run/v1` | Nublar | Immutable collection attempt with check-level provenance, decision state, and optional provider-neutral external correlation. |
| Nublar decision | `ingen.nublar-decision/v1` | Nublar | Provider-neutral delivery projection without producer reports. |
| Nublar delivery receipt | `ingen.nublar-delivery-receipt/v1` | Nublar | One delivery attempt outcome, optionally preserved in a separate local receipt store, separate from the run decision. |
| Sentinel lifecycle receipt | `ingen.sentinel-run/v1` | Sentinel | Workspace lifecycle and opaque artifact lineage around a verifier handoff. |
| Herdr lifecycle event | `ingen.herdr-event/v1` | Sentinel adapter | Idempotent host-event ingress bound to one Sentinel run and workspace. |
| Sentinel CI explanation | `ingen.sentinel-ci-explanation/v1` | Sentinel | Closed producer-owned lifecycle, artifact, and optional audit context nested in the shared CI envelope. |

The `v1` labels are alpha interfaces, not a claim that every field is already
ideal. A breaking field or semantic change must be deliberate, documented, and
accompanied by a version or migration decision; it must not be smuggled into a
producer because the current repository happens to be a monorepo.

## Cross-boundary invariants

These are the important guarantees of the current slice:

1. A campaign plan refers to the exact contract, oracle, catalogue, baseline,
   and policy identities used to create it.
2. A provider is reviewed before a subject is launched. Strict providers must
   bind to the plan hash.
3. Preparation must prove that the provider, plan, summary, source copy, and
   prepared binaries agree. Ambiguous or missing source targets and no-op
   mutations are preparation failures.
4. Every mutation is compared with the same clean baseline and frozen oracle.
   `killed` demonstrates contract sensitivity; it does not demonstrate that
   the contract is complete.
5. `ingen.ci-result/v1` owns orchestration status and exit-code meaning. The
   producer owns the nested report and explanation.
6. Nublar composes severity without interpreting Sorna findings:

   | Inputs | Aggregate | Exit code |
   | --- | --- | ---: |
   | all `passed` | `passed` | 0 |
   | at least one `failed`, no `error` | `failed` | 1 |
   | at least one `error` | `error` | 2 |

7. A path and SHA-256 identify bytes consumed by a run. They establish
   integrity and detect drift; they do not prove correctness, isolation, or
   that an agent never saw implementation details.
8. Nublar loads only closed v1 envelope and run shapes, preserves producer
   reports opaquely, and never changes a stored run decision when delivery
   fails.
9. Nublar run-list status, workflow, and correlation filters are read-only
   projections over validated records; filtering preserves the store's
   deterministic ordering.
10. Nublar correlation metadata is optional, requires a system/ID/positive
    attempt tuple when present, and never replaces the immutable `run_id`.
11. Nublar delivery receipts are independent audit records; storing a failed
    receipt does not change the run decision or create a retry.
12. Receipt-list run/status/transport filters are read-only projections over
    validated receipts and preserve receipt-store ordering.
13. Sentinel Herdr events bind to the active run and workspace, require
    monotonic timestamps, verify referenced artifact bytes when a root is
    supplied, and publish batches only after every event validates.
14. Sentinel event identity is durable at the receipt boundary: an identical
    source-event retry is a no-op, while reuse of an event ID with different
    content is rejected. Terminal receipts cannot reopen; cleanup is the
    explicit terminal-to-`cleaned` exception.
15. Sentinel lifecycle status maps to the shared envelope without ambiguity:
    `completed` is `passed/0`, `failed` is `failed/1`, and blocked or incomplete
    lifecycle states are `error/2`. Nublar composes that envelope status and
    preserves the Sentinel report opaquely.
16. A terminal Sentinel envelope emitted by the CLI is based on one validated
    receipt snapshot and passes the receipt/artifact integrity audit first, so
    drift cannot become a passing Nublar check.
17. A replay matrix records explicit expected classifications, preserves each
    member envelope by path and hash, and passes only when every nested replay
    result matches its declared expectation. Independent verification rechecks
    those member bytes, reports, explanations, lineage, and—when present—the
    manifest that declared the expectations.
18. Sentinel file references are relative to the supplied project root;
    workspace bootstrap, capability-plan loading, artifact registration, Sorna
    handoff, Herdr ingress, and terminal audit resolve symlinks and reject
    paths that escape that root before creating, mutating, or passing an
    artifact reference.
19. The verifier subject root is resolved as an existing directory under the
    supplied project root before Sorna is launched; an escaping, unresolved,
    or non-directory subject root is a preparation failure, not a delegated
    execution. When omitted, it is `.` relative to the child process root.
20. The verifier evidence output directory is relative to the supplied project
    root; existing symlink components are resolved before Sorna is launched,
    and an escaping or existing non-directory output path is a preparation
    failure. A not-yet-created output leaf is permitted only when its existing
    parent remains rooted.
21. A verifier completion artifact discovered under the supplied project root
    is registered with a root-relative reference and its bytes are read through
    that same root; caller working-directory changes cannot silently omit or
    rebind Sorna's `run.json`.
22. The workspace capability compiler and the oracle/verifier CLI handoffs
    resolve the manifest and its policy references in the supplied project-root
    namespace; the caller's working directory is not an alternate source of
    capability-plan input bytes.
23. Sentinel bootstrap accepts a supplied project root, loads the workspace
    manifest from that rooted namespace, and records its path relative to that
    root; the caller's working directory cannot substitute the receipt's
    workspace bytes.
24. Sentinel artifact registration accepts the same supplied project root and
    hashes the referenced file from that namespace before publishing the
    receipt update; an artifact path is never implicitly resolved from the
    caller's working directory.

## Executable evidence index

The frozen invariants are backed by focused tests in the owning package:

| Invariants | Primary executable evidence |
| --- | --- |
| 1–4 | [Sorna contract tests](sorna/internal/contract/contract_test.go), [oracle tests](sorna/internal/oracle/oracle_test.go), [runner tests](sorna/internal/runner/runner_test.go), and [mutation tests](sorna/internal/mutation/mutation_test.go) |
| 5 | [Shared CI-result tests](core/ciresult/ciresult_test.go) |
| 6, 8–12 | [Nublar aggregate tests](nublar/internal/aggregate/aggregate_test.go), [run tests](nublar/internal/run/run_test.go), [delivery projection tests](nublar/internal/delivery/projection_test.go), and [CLI collection/delivery tests](nublar/cmd/nublar/main_test.go) |
| 7 | [Sentinel receipt tests](herdr-sentinel/internal/run/run_test.go) and [Nublar artifact tests](nublar/internal/artifact/artifact_test.go) |
| 13–14 | [Herdr adapter tests](herdr-sentinel/internal/adapter/herdr_test.go), [Sentinel CLI adapter tests](herdr-sentinel/cmd/sentinel/main_test.go), and [receipt update-lock tests](herdr-sentinel/internal/run/update_test.go) |
| 15–16 | [Sentinel CI-result tests](herdr-sentinel/internal/run/ciresult_test.go), [audit tests](herdr-sentinel/internal/audit/audit_test.go), [report tests](herdr-sentinel/internal/report/report_test.go), and [Nublar Sentinel integration tests](nublar/integration/mixed_producers_test.go) |
| 17 | [Sorna replay-matrix tests](sorna/internal/evidence/replay_matrix_test.go) |
| 18–24 | [Rooted receipt tests](herdr-sentinel/internal/run/run_test.go), [rooted adapter tests](herdr-sentinel/internal/adapter/adapter_test.go), [rooted Herdr-event tests](herdr-sentinel/internal/adapter/herdr_test.go), [capability-root tests](herdr-sentinel/internal/capability/capability_test.go), and [Sentinel CLI root tests](herdr-sentinel/cmd/sentinel/main_test.go) |

The index establishes local executable coverage. It does not claim native Herdr
callback authentication, host-owned restart recovery, or shutdown semantics;
those remain outside the repository until the host contract is supplied.

## What is deliberately not frozen

- The complete mutation-operator vocabulary and language-specific target
  model.
- Additional source providers and SDKs. The provider handoff is language
  neutral, but the Go provider is the only production-style provider currently
  exercised end to end.
- A cryptographic or independently attested proof of the oracle writer's
  non-observation of implementation details. The current isolation and
  telemetry are evidence toward that goal, not that proof.
- Sentinel's contract workspace, agent lifecycle, permissions, and user
  experience. Those belong to the Herdr-side workflow surface.
- Nublar as a standalone hosted product. The current Nublar code is a thin
  coordinator and integration proof surface.
- Producer-owned internal evidence schemas such as `sorna.evidence/v1`,
  `ingen.run/v1`, and `sorna.mutation-campaign-explanation/v1`. They are useful
  artifacts, but consumers should use the shared CI envelope rather than
  importing their internal meanings. Sentinel's nested explanation is listed
  above because its closed shape is part of the explicit envelope contract;
  its fields still do not become Nublar semantics.

## Current verification command

Run the Nublar-only boundary check with:

```sh
make nublar-check
```

The provider-neutral consumer example has its own fixture-backed check:

```sh
make nublar-consumer-check
```

Run the focused boundary check with:

```sh
make alpha-interface-check
```

Run the complete clean document workflow with:

```sh
make nublar-aggregate-fresh
make nublar-run-collect-fresh
```

Run the Sentinel positive and expected-failure collection proofs with:

```sh
make nublar-sentinel-run-collect-fresh
make nublar-sentinel-run-collect-external-root-fresh
make nublar-sentinel-run-collect-failure-fresh
make nublar-sentinel-run-collect-external-root-failure-fresh
```

The failure proof returns success only after observing the expected Nublar
`failed/1` decision; it does not hide an unexpected collection error.

These targets are the executable proofs for the current slice: they prepare
the strict Go provider, emit provider-review and preparation CI results,
execute the campaign, and aggregate or persist four required checks in a
temporary workspace.

The full repository check remains separate because the unrelated Paddock
acceptance test currently times out while building its CLI; the focused command
covers the alpha surfaces named here.

## Related specifications

- [CI result envelope](core/CI-RESULT-SPEC.md)
- [Sorna mutation specification](sorna/MUTATION-SPEC.md)
- [Nublar README](nublar/README.md)
- [Nublar run artifact](nublar/RUN-ARTIFACT.md)
- [Nublar delivery boundary](nublar/DELIVERY-BOUNDARY.md)
- [Project status](status.md)
