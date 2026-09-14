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

go run ./paddock/cmd/paddock baseline \
  . \
  --policy paddock.yaml \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace \
  --adapter-arg . \
  --output paddock-baseline.json

go run ./paddock/cmd/paddock check \
  paddock/examples/services/modular-monolith-go/violating \
  --policy paddock/examples/modular-monolith.yaml \
  --baseline paddock-baseline.json

go run ./paddock/cmd/paddock graph \
  paddock/examples/services/feature-sliced-ts/good \
  --policy paddock/examples/feature-sliced-frontend.yaml \
  --format json

go run ./paddock/cmd/paddock graph \
  . \
  --policy paddock.yaml \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace \
  --adapter-arg . \
  --format json > paddock-graph.json

go run ./paddock/cmd/paddock check \
  paddock/examples/services/hexagonal-go/good \
  --policy paddock/examples/hexagonal.yaml \
  --graph paddock-graph.json

go run ./paddock/cmd/paddock ci \
  . \
  --policy-lock paddock.lock.json \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace \
  --adapter-arg . \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json

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

go run ./paddock/cmd/paddock explain paddock-ci-result.json

go run ./paddock/cmd/paddock explain paddock-ci-result.json \
  --status blocking --format json

go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/hexagonal.yaml \
  --cases paddock/examples/hexagonal.policy-tests.yaml
```

Use `--format json` for machine-readable findings. A violating subject exits
with status 1; an invalid policy, unreadable source, or incompatible baseline
exits with status 2. Baselines retain accepted findings in the report, mark
them as non-blocking, and report entries that have become stale.

`explain` consumes either a `paddock.report/v1` report or an
`ingen.ci-result/v1` CI artifact and produces deterministic text or JSON with
the observed dependency, matched rule constraints, policy reason, finding
status, and suggested remediation. It also includes per-rule counts for total,
active, blocking, waived, and baselined findings, plus an explicit triage
outcome of `remediate`, `review`, `accepted`, or `clear`. This allows an LLM
agent or CI reviewer to triage a large report before inspecting each edge. It
never changes the underlying verdict. Use `--rule <id>` to select one rule, or
`--status all|active|blocking|advisory|waived|baselined|expired-waiver` to
select a finding state. `blocking` selects active error findings, including
expired waivers. Filtered output records its selection and retains the original
overall verdict.

`graph` exposes the adapter output before classification and rule evaluation.
It accepts either `--policy` or an explicit `--language`, and emits the stable
`paddock.graph/v1` shape for tools and agents that need to inspect the graph.
The same shape can be supplied back to `check` or `ci` with `--graph`, allowing
an external language adapter to provide the graph without being compiled into
Paddock. The policy language and source unit must match the graph document.
An external adapter can be invoked with `--adapter` and repeated
`--adapter-arg` flags; its stdin/stdout contract is documented in
[`ADAPTER-PROTOCOL.md`](ADAPTER-PROTOCOL.md).
`check` can invoke an adapter directly. `ci` can do the same when
`--graph-output` names the durable graph file whose hash is recorded in the
CI artifact.

`init` creates a deterministic draft policy from the current graph. It groups
source units by directory, applies conservative template role guesses, and
writes all generated rules as warnings. The output is intentionally not an
architecture verdict and will not overwrite an existing file without
`--force`. It accepts `--graph` or `--adapter` for languages outside the
built-in adapter registry; generic layered and cyclic drafts remain available
for those languages.

`policy diff` validates both policy files and compares their normalized
semantics. Its `paddock.policy-diff/v1` JSON output includes raw and canonical
input hashes plus stable changes to source settings, components, rules, and waivers. It is the
review boundary for agent-proposed policy edits; it does not apply or approve
the proposal.

`policy test` runs a policy against a versioned case manifest using the same
checker as `check` and `ci`. The `paddock.policy-tests/v1` manifest accepts
expected `pass`, `fail`, or `error` outcomes. A case mismatch exits `1`; an
invalid policy or manifest exits `2`. Use `--format json` for an agent-facing
`paddock.policy-test-result/v1` document.

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

go run ./paddock/cmd/paddock init \
  . \
  --language rust \
  --unit file \
  --adapter ./tools/paddock-language-adapter \
  --output paddock.yaml
```

`ci` writes the language-neutral `ingen.ci-result/v1` envelope defined in
[`core/CI-RESULT-SPEC.md`](../core/CI-RESULT-SPEC.md). It includes the
deterministic report, explanation, policy hash, optional policy-lock hash,
optional graph hash, source identity, and the same exit code that the CI gate
receives.

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
Baseline generation also accepts `--graph` or `--adapter`, so external-language
projects can adopt the same “no new violations” workflow.

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
External adapter authors can use the machine-readable contracts in
[`spec/`](spec/) and the pass-through conformance fixture in
[`examples/adapter/`](examples/adapter/README.md).
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
