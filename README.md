# InGen

> “Your scientists were so preoccupied with whether or not they could, they didn’t stop to think if they should.”
>
> — Dr. Ian Malcolm, *Jurassic Park*

InGen is an ecosystem for independent contract verification and agent-assisted
software development.

The implementation is intentionally a language-aware monorepo. The named
verticals are product surfaces around shared artifacts and protocols; they are
not yet separate services or repositories.

- **Herdr Sentinel** — the Herdr workflow and control-room surface: contract
  workspaces, agent roles, permissions, lifecycle, evidence handoffs, and
  visibility.
- **Malcolm** — the executable specification surface: a Rust parser, semantic
  validator, language-neutral JSON IR, CLI, and first Sorna bridge.
- **Hammond** — the contract governance and registry surface: approvals,
  amendments, authority, membership, trust, and contract lineage.
- **Paddock** — the architecture-fence surface: language-neutral dependency
  graphs, architecture policies, reviews, locks, explanations, and CI gates.
- **Sorna** — the standalone verification laboratory: contracts, isolated
  oracles, black-box runs, mutations, replay, and evidence.
- **Amber** — the portable provenance layer: Go and TypeScript SDKs for work
  identity, execution tracking, causality, attribution, retries, replay, and
  safe context propagation.
- **Lockwood** — the evidence-custody and artifact-registry surface:
  content-addressed artifacts, custody records, lineage, handling events,
  integrity, and detached attestations.
- **Nublar** — the CI and delivery surface: shared result aggregation,
  persistent run collection, queries, delivery receipts, and consumer gates.
- **Sattler** — the verification intelligence surface: comparisons,
  campaign summaries, provenance/custody correlation, and regression analysis.

## Core thesis

When an implementation and its tests are produced from the same implementation-informed context, they can form a closed circle. The tests may pass because they agree with the code, not because the code satisfies the intended behaviour.

The family is designed to break that circle with three separations:

1. **The contract is independent.** It is reviewed and frozen before test generation and implementation evaluation.
2. **The oracle writer is isolated.** The test-writing actor must not be able to inspect the implementation.
3. **The result is challenged.** Mutation testing and deliberate contract violations check whether the resulting tests are sensitive to wrong behaviour.

## Product relationship

The shared core defines contract, run, mutation, evidence, and CI-result
protocols. Malcolm describes executable specifications. Hammond governs their
ownership and approval. Paddock evaluates architecture independently. Sentinel
coordinates agent workspaces and lifecycle. Sorna owns verification semantics,
mutation execution, replay, and evidence production. Amber provides reusable
provenance semantics across the ecosystem. Lockwood preserves artifacts and
custody history. Nublar coordinates CI and delivery without reimplementing
producer semantics. Sattler compares the resulting history and reports
regressions without rewriting producer verdicts.

The intended relationship is:

```text
Malcolm specification + Hammond governance + Paddock policy
        -> Sentinel workflow and Nublar coordination
        -> Sorna verification, mutation, replay, and evidence
        -> Amber provenance + Lockwood custody
        -> Sattler comparison and regression analysis
```

The separation that must be enforced is the oracle's runtime access boundary.
The separation between named verticals is otherwise allowed to remain a
logical boundary inside one codebase until independent deployment or ownership
becomes useful.

## Current status

The current local checkpoint is the v0.9.2 development release. The repository
now contains working slices across the full verification chain:

- Sorna runs sealed contracts, isolated subjects, mutation campaigns, evidence
  replay, replay matrices, and CI-facing results.
- Malcolm parses and validates executable specifications, emits a stable
  language-neutral JSON IR, and can lower request/stateful examples into Sorna
  contracts and runs.
- Paddock provides a compiled, language-neutral architecture gate with Go,
  TypeScript/JavaScript, Python, and external-adapter support.
- Hammond provides local contract governance with authority, membership,
  trust-root, approval, amendment, and lineage boundaries.
- Lockwood provides local content-addressed custody, integrity checks, handling
  events, artifact lineage, and detached attestation workflows.
- Sentinel provides workspace/capability orchestration, Herdr event ingress,
  receipt locking, artifact registration, audit, reporting, and Nublar handoff.
- Nublar aggregates producer results, stores and queries run receipts, projects
  delivery decisions, and has a provider-neutral consumer gate.
- Amber provides Go and TypeScript provenance SDKs, adapters, storage seams,
  and cross-language conformance fixtures.
- Sattler compares CI results and can correlate changes across producer inputs,
  Nublar runs, custody artifacts, and provenance records.

Assurance remains deliberately conservative: oracle and subject telemetry,
provenance, custody, and signatures establish explicit recorded boundaries but
do not automatically constitute independent attestation or universal
correctness.

See [MODULES.md](MODULES.md) for the repository map and the intended status of
each area.

See [roadmap.md](roadmap.md) for the overall InGen checkpoint, completed
surfaces, explicit non-claims, and candidate next phases.

The current cross-tool alpha boundary is recorded in
[ALPHA-INTERFACES.md](ALPHA-INTERFACES.md). The focused verification command
is `make alpha-interface-check`; the complete clean workflow is
`make nublar-aggregate-fresh`.

Amber now has a language-neutral v1 specification, Go and TypeScript
implementations, shared conformance fixtures, and focused transition tests.

The current cross-vertical freeze surfaces are documented in
[`nublar/FREEZE-RECORD.md`](nublar/FREEZE-RECORD.md) and
[`ALPHA-INTERFACES.md`](ALPHA-INTERFACES.md). The current implementation
status and known limitations are tracked in [`status.md`](status.md).

## Quick start

```sh
make help
make check
make sorna-alpha-check
make contract-validate
make contract-seal
make policy-validate
make sandbox-contract-read
make sorna-oracle-freeze
make oracle-evidence-verify
make sorna-run
make evidence-verify
make sorna-ci-result
make nublar-aggregate
make nublar-aggregate-fresh
make nublar-freeze-check
make nublar-consumer-check
make sorna-replay-matrix-ci-result-fresh
make malcolm-sorna-flow-run
```

`make sorna-run` freezes the oracle, launches the clean subject, waits for
`GET /healthz`, runs from `oracle.json`, writes an evidence bundle, and tears
the subject down. The
`make evidence-verify` target checks its recorded artifact hashes. The lower-level
`make subject-run` plus `make sorna-external-run` targets remain available when
you need to supply an already-running subject yourself.

The managed subject listens on `127.0.0.1:8080` by default. Use a matching
custom `SUBJECT_POLICY` when choosing another port, for example
`make sorna-run SUBJECT_ADDR=127.0.0.1:8081 SUBJECT_URL=http://127.0.0.1:8081`.

To exercise the first controlled defect, run `make sorna-defect-run`. The
output should show a failing contract verdict and a `killed` mutation outcome.
In mutation mode, the command exits successfully when the declared mutation is
killed.

`make sorna-ci-result` now includes the clean managed baseline run, so it can
also start from an empty artifact root. Generated artifacts default to
`.artifacts`. For a clean, reviewable run with no stale-output collisions, use
`make nublar-aggregate-fresh`; it snapshots the current source tree into a new
temporary workspace, runs with the normal workspace-relative policies, and
prints the workspace and artifact locations when the workflow ends. The
`ARTIFACT_ROOT` variable can also relocate outputs when the selected policy
paths cover that location.

To inspect two shared CI results with Sattler:

```sh
go run ./sattler/cmd/sattler compare before.json after.json
go run ./sattler/cmd/sattler compare --format json before.json after.json
```
