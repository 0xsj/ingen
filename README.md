# InGen

> “Your scientists were so preoccupied with whether or not they could, they didn’t stop to think if they should.”
>
> — Dr. Ian Malcolm, *Jurassic Park*

InGen is an ecosystem for independent contract verification and agent-assisted
software development.

The initial implementation is intentionally a Go-oriented monorepo. The named
verticals are product surfaces around shared artifacts and protocols; they are
not yet separate services or repositories.

- **Sorna** — the standalone verification laboratory: contracts, isolated
  oracles, black-box runs, mutations, and evidence.
- **Herdr Sentinel** — the Herdr workflow and control-room surface: contract
  workspaces, agent roles, permissions, lifecycle, and visibility.
- **Hammond** — a future contract governance and registry surface, currently a
  placeholder until cross-project governance is a real need.
- **Nublar** — the CI and delivery surface: currently a local coordinator for
  shared result envelopes, with hosted workflow capabilities still ahead.
- **Amber** — the portable provenance layer: Go and TypeScript SDKs for work
  identity, execution tracking, causality, attribution, retries, replay, and
  safe context propagation.

## Core thesis

When an implementation and its tests are produced from the same implementation-informed context, they can form a closed circle. The tests may pass because they agree with the code, not because the code satisfies the intended behaviour.

The family is designed to break that circle with three separations:

1. **The contract is independent.** It is reviewed and frozen before test generation and implementation evaluation.
2. **The oracle writer is isolated.** The test-writing actor must not be able to inspect the implementation.
3. **The result is challenged.** Mutation testing and deliberate contract violations check whether the resulting tests are sensitive to wrong behaviour.

## Product relationship

The shared core defines contract, run, mutation, and evidence artifacts. Sorna
owns verification semantics and evidence production. Sentinel owns the agent
workflow and interactive workspace. Amber provides reusable provenance
semantics across the ecosystem. Nublar consumes Sorna and shared result
protocols rather than duplicating their testing logic.

The separation that must be enforced is the oracle's runtime access boundary.
The separation between named verticals is otherwise allowed to remain a
logical boundary inside one codebase until independent deployment or ownership
becomes useful.

## Current status

The design specifications and initial repository skeleton are in place. Sorna
now has a first Go contract foundation, a public-boundary runner, managed
subject lifecycle, a controlled defect detected by the sealed contract, and a
macOS Seatbelt-backed workflow that freezes an oracle before running a subject.
Managed subjects can now use a separate host-enforced policy, compiled runtime
binary, and subject access stream. Assurance remains deliberately conservative:
oracle and subject telemetry are evidence, not independent attestation.

See [MODULES.md](MODULES.md) for the repository map and the intended status of
each area.

The current cross-tool alpha boundary is recorded in
[ALPHA-INTERFACES.md](ALPHA-INTERFACES.md). The focused verification command
is `make alpha-interface-check`; the complete clean workflow is
`make nublar-aggregate-fresh`.

Amber now has a language-neutral v1 specification, Go and TypeScript
implementations, shared conformance fixtures, and focused transition tests.

## Quick start

```sh
make help
make check
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
