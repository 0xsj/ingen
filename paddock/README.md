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

The implemented Go commands are the checker, baseline generator, and report
explainer:

```sh
go run ./paddock/cmd/paddock check \
  paddock/examples/services/hexagonal-go/good \
  --policy paddock/examples/hexagonal.yaml

go run ./paddock/cmd/paddock baseline \
  paddock/examples/services/modular-monolith-go/violating \
  --policy paddock/examples/modular-monolith.yaml \
  --output paddock-baseline.json

go run ./paddock/cmd/paddock check \
  paddock/examples/services/modular-monolith-go/violating \
  --policy paddock/examples/modular-monolith.yaml \
  --baseline paddock-baseline.json

go run ./paddock/cmd/paddock explain paddock-report.json
```

Use `--format json` for machine-readable findings. A violating subject exits
with status 1; an invalid policy, unreadable source, or incompatible baseline
exits with status 2. Baselines retain accepted findings in the report, mark
them as non-blocking, and report entries that have become stale.

`explain` consumes a JSON report and produces deterministic text or JSON with
the observed dependency, matched rule constraints, policy reason, finding
status, and suggested remediation. It is safe for an LLM agent to consume, but
it never changes the underlying verdict.

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

The first executable Go slice is implemented. It loads a policy, builds a
package/import graph, classifies packages, applies the initial rule set, and
emits text or JSON reports with deterministic exit codes.

Policies can also carry reviewable waivers. Active waivers are visible in the
report but do not block the check; expired waivers remain blocking.

Existing violations can be captured in a `paddock.baseline/v1` snapshot. A
baseline is tied to the source module, uses stable finding identities, and does
not hide new findings. Stale entries are reported so the snapshot can be
cleaned up as the architecture improves.

The service fixtures are in [`examples/services/`](examples/services/README.md).
The acceptance suite runs every good and violating subject through the CLI,
including a deliberately cyclic Go subject, and verifies invalid-policy
failures and the machine-readable report shape.

Run the focused suite with:

```sh
GOCACHE=.cache/paddock-go-build go test ./paddock/...
```

The current adapter is Go-only. The policy schema is language-neutral, so
future TypeScript or other adapters should produce the same graph model rather
than change the rule engine.

## Design principle

Paddock should learn the desired architecture from an explicit policy, not infer
it from the existing code and then declare the existing code correct.
