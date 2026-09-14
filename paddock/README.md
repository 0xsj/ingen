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

## CLI

```sh
paddock init
paddock check .
paddock graph .
paddock policy diff --before paddock.yaml --after proposed-paddock.yaml
paddock policy seal --input paddock.yaml --output paddock.lock.json
paddock explain paddock-report.json
paddock baseline
paddock ci . --policy paddock.yaml --output paddock-ci-result.json
```

The implemented Go commands are the checker, graph inspector, baseline
generator, report explainer, and CI artifact producer:

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

go run ./paddock/cmd/paddock graph \
  paddock/examples/services/feature-sliced-ts/good \
  --policy paddock/examples/feature-sliced-frontend.yaml \
  --format json

go run ./paddock/cmd/paddock policy diff \
  --before paddock.yaml \
  --after proposed-paddock.yaml \
  --format json

go run ./paddock/cmd/paddock policy seal \
  --input paddock.yaml \
  --output paddock.lock.json

go run ./paddock/cmd/paddock check \
  paddock/examples/services/hexagonal-go/good \
  --policy paddock/examples/hexagonal.yaml \
  --policy-lock paddock.lock.json

go run ./paddock/cmd/paddock check \
  paddock/examples/services/hexagonal-go/good \
  --policy-lock paddock.lock.json

go run ./paddock/cmd/paddock explain paddock-report.json

go run ./paddock/cmd/paddock ci \
  paddock/examples/services/hexagonal-go/violating \
  --policy paddock/examples/hexagonal.yaml \
  --output paddock-ci-result.json
```

Use `--format json` for machine-readable findings. A violating subject exits
with status 1; an invalid policy, unreadable source, or incompatible baseline
exits with status 2. Baselines retain accepted findings in the report, mark
them as non-blocking, and report entries that have become stale.

`explain` consumes a JSON report and produces deterministic text or JSON with
the observed dependency, matched rule constraints, policy reason, finding
status, and suggested remediation. It is safe for an LLM agent to consume, but
it never changes the underlying verdict.

`graph` exposes the adapter output before classification and rule evaluation.
It accepts either `--policy` or an explicit `--language`, and emits the stable
`paddock.graph/v1` shape for tools and agents that need to inspect the graph.

`init` creates a deterministic draft policy from the current graph. It groups
source units by directory, applies conservative template role guesses, and
writes all generated rules as warnings. The output is intentionally not an
architecture verdict and will not overwrite an existing file without
`--force`.

`policy diff` validates both policy files and compares their normalized
semantics. Its `paddock.policy-diff/v1` JSON output includes raw and canonical
input hashes plus stable changes to source settings, components, rules, and waivers. It is the
review boundary for agent-proposed policy edits; it does not apply or approve
the proposal.

`policy seal` writes a `paddock.policy-lock/v1` artifact containing the exact
policy-file SHA-256, canonical semantic SHA-256, and canonical policy payload.
`policy verify` checks both hashes against a policy file. The optional
`--policy-lock` flag on `check` and `ci` validates the lock before evaluation;
when `--policy` is also supplied, it additionally verifies both hashes against
that file. The exact-file binding is intentional: even a comment or formatting
change requires a new seal. When `--policy-lock` is used without `--policy`,
Paddock evaluates the canonical policy embedded in the lock; the original
policy file is not required to be present.

```sh
go run ./paddock/cmd/paddock init \
  paddock/examples/services/layered-go/good \
  --template layered \
  --output paddock.yaml
```

`ci` writes the language-neutral `ingen.ci-result/v1` envelope defined in
[`core/CI-RESULT-SPEC.md`](../core/CI-RESULT-SPEC.md). It includes the
deterministic report, explanation, policy hash, optional policy-lock hash,
source identity, and the same exit code that the CI gate receives.

The CI verdict must be deterministic. An LLM may propose policies, explain
findings, and suggest migrations, but it must not decide whether a build passes.

A provider-neutral adoption example is in
[`examples/ci/`](examples/ci/README.md). It keeps proposal review and sealing
explicit, while ordinary CI runs only the lock-backed gate.

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
report but do not block the check; expired waivers remain blocking. Waivers
that match no current finding are reported as unused for cleanup.

Existing violations can be captured in a `paddock.baseline/v1` snapshot. A
baseline is tied to the source module and canonical policy SHA-256, uses stable
finding identities, and does not hide new findings. Formatting-only changes do
not require regeneration; semantic policy changes do. Stale entries are
reported so the snapshot can be cleaned up as the architecture improves.

The service fixtures are in [`examples/services/`](examples/services/README.md).
The acceptance suite runs every good and violating subject through the CLI,
including a deliberately cyclic Go subject, and verifies invalid-policy
failures and the machine-readable report shape.

Run the focused suite with:

```sh
GOCACHE=.cache/paddock-go-build go test ./paddock/...
```

The current adapters support Go, TypeScript/JavaScript, and Python. All
implement the same adapter registry contract and expose capabilities for source
units and edge kinds. The policy schema is language-neutral, so additional
adapters should produce the same graph model rather than change the rule engine.
The Python adapter resolves absolute and relative project modules without
importing or executing project code, and classifies standard-library,
third-party, and unresolved imports for policy rules.
The TypeScript adapter resolves relative imports and `tsconfig.json` path
aliases, including local inherited JSONC configs, without requiring npm or a project
build, and can report unresolved imports explicitly.

Rules default to blocking `error` findings, but may declare `warning` or `info`
severity for non-blocking architectural guidance. Required dependencies are
direct by default and can opt into transitive reachability with
`transitive: true`; cycle checks are scoped by source roots and optional rule
selectors.

## Design principle

Paddock should learn the desired architecture from an explicit policy, not infer
it from the existing code and then declare the existing code correct.
