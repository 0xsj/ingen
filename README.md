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
- **Nublar** — a future CI and delivery surface, currently a placeholder that
  should consume Sorna rather than duplicate its semantics.

## Core thesis

When an implementation and its tests are produced from the same implementation-informed context, they can form a closed circle. The tests may pass because they agree with the code, not because the code satisfies the intended behaviour.

The family is designed to break that circle with three separations:

1. **The contract is independent.** It is reviewed and frozen before test generation and implementation evaluation.
2. **The oracle writer is isolated.** The test-writing actor must not be able to inspect the implementation.
3. **The result is challenged.** Mutation testing and deliberate contract violations check whether the resulting tests are sensitive to wrong behaviour.

## Product relationship

The shared core defines contract, run, mutation, and evidence artifacts. Sorna
owns verification semantics and evidence production. Sentinel owns the agent
workflow and interactive workspace. Future CI or registry surfaces should
invoke or consume Sorna rather than duplicate its testing logic.

The separation that must be enforced is the oracle's runtime access boundary.
The separation between named verticals is otherwise allowed to remain a
logical boundary inside one codebase until independent deployment or ownership
becomes useful.

## Current status

The design specifications and initial repository skeleton are in place. Sorna
now has a first Go contract foundation, a public-boundary runner, managed
subject lifecycle, and a controlled defect that is detected by the sealed
contract. The next implementation step is to turn lifecycle evidence into a
real evidence bundle and then introduce capability enforcement.

See [MODULES.md](MODULES.md) for the repository map and the intended status of
each area.

## Quick start

```sh
make help
make check
make contract-validate
make contract-seal
make sorna-run
make evidence-verify
```

`make sorna-run` launches the clean subject, waits for `GET /healthz`, runs the
contract, writes an evidence bundle, and tears the subject down. The
`make evidence-verify` target checks its recorded artifact hashes. The lower-level
`make subject-run` plus `make sorna-external-run` targets remain available when
you need to supply an already-running subject yourself.

The managed subject listens on `:8080` by default. Use
`make sorna-run SUBJECT_ADDR=:8081 SUBJECT_URL=http://localhost:8081` to choose
another address.

To exercise the first controlled defect, run `make sorna-defect-run`. The
output should show a failing contract verdict and a `killed` mutation outcome.
In mutation mode, the command exits successfully when the declared mutation is
killed.
