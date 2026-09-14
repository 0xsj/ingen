# Paddock

Paddock is InGen's architecture-fence tool. It turns dependency and layering
decisions into deterministic, reviewable rules that can run locally or in CI.

Paddock is not a Hexagonal Architecture validator with one fixed opinion. It
provides a small policy language that can describe several architectural
shapes:

- layered applications;
- clean and onion architectures;
- hexagonal applications;
- modular monoliths and bounded contexts;
- vertical slices;
- feature-sliced frontends;
- plugin systems;
- migration boundaries.

The core abstraction is:

```text
classify source -> build dependency graph -> constrain relationships
```

Architecture templates are conveniences. The policy remains the authority.

## Planned CLI

```sh
paddock init
paddock check .
paddock graph .
paddock explain paddock-report.json
paddock baseline
```

The CI verdict must be deterministic. An LLM may propose policies, explain
findings, and suggest migrations, but it must not decide whether a build passes.

## InGen relationship

Paddock is source-level architecture verification. Sorna verifies externally
observable behavior through contracts, oracles, and mutations. They can share
artifact identity, evidence, and review conventions, but they are different
verification planes.

- Paddock owns architecture policies, dependency graphs, and structural findings.
- Sorna owns behavioral contracts, oracle execution, mutation semantics, and
  verification evidence.
- Nublar can run both as CI gates.
- Sentinel can show both results in the agent workflow.
- Hammond may later govern policies shared across projects.

## Current state

This directory contains the initial policy-language design, four example
architectures, and small Go service subjects. Each service has a `good/`
baseline and a `violating/` variant so the eventual analyzer can prove both
that it accepts the intended graph and that it catches a named defect.

The service fixtures are in [`examples/services/`](examples/services/README.md).
There is no analyzer implementation yet. The policies and subjects are the
first fixtures against which the policy schema should be tested.

## Design principle

Paddock should learn the desired architecture from an explicit policy, not infer
it from the existing code and then declare the existing code correct.
