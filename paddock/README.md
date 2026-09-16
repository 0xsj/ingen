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
paddock version
paddock init
paddock check .
paddock graph .
paddock map . --policy paddock.yaml
paddock policy validate --policy paddock.yaml
paddock policy diff --before paddock.yaml --after proposed-paddock.yaml
paddock policy seal --input paddock.yaml --output paddock.lock.json
paddock explain paddock-report.json
paddock baseline
paddock ci . --policy paddock.yaml --output paddock-ci-result.json
```

### Build and install

Paddock can be used as a compiled CI binary instead of `go run`:

```sh
make -C paddock build \
  VERSION=0.1.0 \
  COMMIT="$(git rev-parse --short HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

.artifacts/paddock/paddock version
.artifacts/paddock/paddock version --format json
```

Install it into the Go toolchain's bin directory with `make -C paddock install`.
For a release-shaped local check, use `make -C paddock release-check`; it builds
the binary, runs Paddock tests and vet, and validates both version output
formats. Release metadata is injected through `VERSION`, `COMMIT`, and
`BUILD_DATE` variables; development builds default to `dev` and `unknown`.
Cross-platform archives and `SHA256SUMS` are produced with
`make -C paddock release-artifacts`. See [`RELEASE.md`](RELEASE.md) for the
release checklist. That command also writes a versioned
`release-manifest.json` describing each archive and its SHA-256 digest.
Verify a generated bundle with `paddock release verify --manifest
release-manifest.json`; a digest mismatch exits `1` and malformed manifest
input exits `2`.
The current schema identifiers and compatibility rules are documented in
[`COMPATIBILITY.md`](COMPATIBILITY.md); machine-readable definitions are in
[`spec/`](spec/).
Working product boundaries and unresolved design choices are tracked in
[`DECISIONS.md`](DECISIONS.md).

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

go run ./paddock/cmd/paddock policy validate \
  --policy paddock.yaml \
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

go run ./paddock/cmd/paddock policy review \
  --before paddock/examples/hexagonal.yaml \
  --after proposed-paddock.yaml \
  --cases paddock/examples/hexagonal.policy-tests.yaml \
  --output paddock-policy-review.json
```

Use `--format json` for machine-readable findings. A violating subject exits
with status 1; an invalid policy, unreadable source, or incompatible baseline
exits with status 2. Baselines retain accepted findings in the report, mark
them as non-blocking, and report entries that have become stale.
The report contract is defined in
[`spec/paddock.report-v1.schema.json`](spec/paddock.report-v1.schema.json).

`policy validate` performs policy-only validation and prints a concise summary
in text mode. JSON mode emits the normalized, deterministically ordered
`paddock.architecture/v1` policy, which is useful as an agent or CI preflight
input before sealing or checking it. For invalid input, JSON mode emits a
`paddock.policy-validation/v1` diagnostic document and exits `2`.

`explain` consumes either a `paddock.report/v1` report or an
`ingen.ci-result/v1` CI artifact and produces deterministic text or JSON with
the observed dependency, matched rule constraints, policy reason, finding
status, and suggested remediation. It also includes per-rule counts for total,
active, blocking, waived, and baselined findings, plus an explicit triage
outcome of `remediate`, `review`, `accepted`, or `clear`. This allows an LLM
agent or CI reviewer to triage a large report before inspecting each edge. It
never changes the underlying verdict. Deny-rule remediation suggestions include
the normalized denied targets, including internal path and external package
patterns. When multiple rules report the same source-to-target edge, each JSON
finding includes the other rule IDs in `related_rules`, and text mode emits a
matching `Related:` hint. Use `--rule <id>` to select one rule, or
`--status all|active|blocking|advisory|waived|baselined|expired-waiver` to
select a finding state. `blocking` selects active error findings, including
expired waivers. Filtered output records its selection and retains the original
overall verdict. The JSON contract is defined in
[`spec/paddock.explanation-v1.schema.json`](spec/paddock.explanation-v1.schema.json).
When the input is an `ingen.ci-result/v1` artifact, the explanation also
includes optional provenance with the artifact path/hash, recorded status and
exit code, timestamp, and policy, lock, graph, and baseline file references.
This lets an agent correlate a filtered explanation with the exact CI evidence
it came from.

`graph` exposes the adapter output before classification and rule evaluation.
It accepts either `--policy` or an explicit `--language`, and emits the stable
`paddock.graph/v1` shape for tools and agents that need to inspect the graph.
The same shape can be supplied back to `check` or `ci` with `--graph`, allowing
an external language adapter to provide the graph without being compiled into
Paddock. The policy language and source unit must match the graph document.
An external adapter can be invoked with `--adapter` and repeated
`--adapter-arg` flags; its stdin/stdout contract is documented in
[`ADAPTER-PROTOCOL.md`](ADAPTER-PROTOCOL.md).
For repeatable use, `--adapter-config <profile.yaml>` loads a versioned
`paddock.adapter-profile/v1` profile containing the executable and arguments.
Profile arguments may use `{{root}}` for the current source root and
`{{profile_dir}}` for the profile's directory; the explicit executable and
argument flags cannot be combined with a profile.
`check` can invoke an adapter directly. `ci` can do the same when
`--graph-output` names the durable graph file whose hash is recorded in the
CI artifact. Graph evidence also records optional adapter identity and
non-secret invocation digests; CI-derived explanations surface that metadata
through provenance when the graph file is available.
When `graph` is used with `--language` and no policy, Paddock supplies the
language's default source unit (`file` for Python and TypeScript/JavaScript,
otherwise `package`); use `--unit` to override it for an external adapter.

`map` provides a compact review view after policy classification. It emits the
`paddock.component-map/v1` shape with component package counts, cross-component
edge counts, and grouped external or unresolved dependencies. It is an
inspection aid only and never changes the policy verdict; use `--format json`
when an agent or another tool needs the structured map.

`adapter validate` exercises an external adapter and validates its graph
response without evaluating a policy. It is useful as a language-adapter
conformance check during development; valid responses exit `0`, while process,
schema, language, or capability errors exit `2`.
Successful JSON output is the normalized `paddock.graph/v1` document. For an
invalid external adapter, JSON mode emits `paddock.adapter-validation/v1` with
a stable failure code and exits `2`; text mode retains the concise diagnostic.
The same diagnostic is emitted by `graph --format json` when its explicit
external adapter cannot produce a valid graph.

`adapter test` runs a versioned `paddock.adapter-tests/v1` manifest with
multiple roots or adapter modes and emits `paddock.adapter-test-result/v1`
evidence. Use `--output <path>` to persist the result and `adapter test verify`
to validate it later. Use `--ci-result <path>` to also emit a shared
`ingen.ci-result/v1` envelope; `adapter test validate` performs manifest-only
preflight. Each failed case records an optional stable `error_code`, such as
`language-mismatch`, `invalid-graph`, or `process-failure`, alongside the
human-readable error.

The `component-owns` rule adds a package-level ownership assertion. It checks
that selected source units belong to one of the component names or label
selectors listed in `allow`, which is useful for keeping a bounded context from
gaining undeclared component types.

`policy validate` performs semantic rule checks in addition to schema checks:
required options must be present, and options unsupported by a rule kind are
rejected before analysis runs.

`ci validate --input <path>` validates an existing `ingen.ci-result/v1`
artifact without rerunning analysis. It validates the shared envelope and
preserves producer-owned `report` and `explanation` JSON, so the same command
can inspect Paddock results or artifacts emitted by another language. A valid
artifact exits `0` regardless of whether its recorded status is `passed` or
`failed`; malformed artifacts exit `2`.

`init` creates a deterministic draft policy from the current graph. It groups
source units by directory, applies conservative template role guesses, and
writes all generated rules as warnings. The output is intentionally not an
architecture verdict and will not overwrite an existing file without
`--force`. Use `--format json` for a `paddock.init/v1` summary that reports
source-unit and edge counts, unclassified components, warning rules, and the
required human-review state. This gives an agent enough context to judge the
review surface without pretending that the scaffold inferred the intended
architecture.
It accepts `--graph` or `--adapter` for languages outside the
built-in adapter registry; generic layered and cyclic drafts remain available
for those languages.

A simple agent handoff is:

```sh
paddock init ./service --output proposed-paddock.yaml --format json > paddock-init.json
paddock policy validate --policy proposed-paddock.yaml --format json
paddock policy diff --before paddock.yaml --after proposed-paddock.yaml --format json
paddock policy review --before paddock.yaml --after proposed-paddock.yaml --format json
```

The summary describes the draft; `policy validate` checks its shape; `policy
diff` exposes semantic changes; and `policy review` gives the approval-oriented
result. None of these steps silently approves or applies a draft.

`policy diff` validates both policy files and compares their normalized
semantics. Its `paddock.policy-diff/v1` JSON output includes raw and canonical
input hashes plus stable changes to source settings, components, rules, and waivers. It is the
review boundary for agent-proposed policy edits; it does not apply or approve
the proposal. Add `--cases <manifest.yaml>` to evaluate the proposed `--after`
policy against a `paddock.policy-tests/v1` manifest. The test results are
embedded in the diff, and a mismatched case exits `1`.
If either policy cannot be loaded, JSON mode emits the
`paddock.policy-validation/v1` diagnostic document and exits `2`. The
diagnostic contains every independent validation issue, with a stable code and
policy path, rather than stopping at the first issue.
The diff contract is defined in
[`spec/paddock.policy-diff-v1.schema.json`](spec/paddock.policy-diff-v1.schema.json).

`policy test` runs a policy against a versioned case manifest using the same
checker as `check` and `ci`. The `paddock.policy-tests/v1` manifest accepts
expected `pass`, `fail`, or `error` outcomes. A case mismatch exits `1`; an
invalid policy or manifest exits `2`. Use `--format json` for an agent-facing
`paddock.policy-test-result/v1` document. External-language cases can pass the
same `--adapter` and repeated `--adapter-arg` options as `check`.
Cases can also use `require_rules` to assert that a failure is attributed to
specific rule IDs rather than merely observing any failure. JSON results also
include the deterministic `finding_rules` list for each evaluated case.
The result contract is defined in
[`spec/paddock.policy-test-result-v1.schema.json`](spec/paddock.policy-test-result-v1.schema.json).
The manifest contract is defined in
[`spec/paddock.policy-tests-v1.schema.json`](spec/paddock.policy-tests-v1.schema.json).
Use `paddock policy test validate --cases <manifest.yaml>` for a manifest-only
preflight that does not load a policy or inspect source code.

`policy review` writes a durable `paddock.policy-review/v1` JSON artifact that
bundles the before/after policy diff and the proposed policy's test results.
The artifact is suitable for pull-request or agent evidence; a failed case
returns exit code `1`. If either policy cannot be loaded, JSON mode emits the
same structured validation diagnostic instead of creating an incomplete review
artifact. Validate a saved artifact with
`paddock policy review verify --input paddock-policy-review.json`; add `--files`
to verify the recorded policy and manifest hashes against the current files.
When `--format json` is used, stdout contains only the review document so it
can be consumed directly by an agent or CI step.
The review artifact contract is defined in
[`spec/paddock.policy-review-v1.schema.json`](spec/paddock.policy-review-v1.schema.json).

`policy seal` writes a `paddock.policy-lock/v1` artifact containing the exact
policy-file SHA-256, canonical semantic SHA-256, and canonical policy payload.
`policy verify` checks both hashes against a policy file. The optional
`--policy-lock` flag on `check` and `ci` validates the lock before evaluation;
when `--policy` is also supplied, it additionally verifies both hashes against
that file. The exact-file binding is intentional: even a comment or formatting
change requires a new seal. When `--policy-lock` is used without `--policy`,
Paddock evaluates the canonical policy embedded in the lock; the original
policy file is not required to be present.
The lock contract is defined in
[`spec/paddock.policy-lock-v1.schema.json`](spec/paddock.policy-lock-v1.schema.json).

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
The shared envelope schema is
[`core/ciresult-v1.schema.json`](../core/ciresult-v1.schema.json).

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

The executable development slice is implemented. Paddock loads a policy,
builds or consumes a dependency graph, classifies source units, applies the
rule set, and emits reviewable text or JSON artifacts with deterministic exit
codes. Built-in Go, TypeScript/JavaScript, and Python adapters share the same
policy engine, while external adapters use the versioned graph protocol.

Policies can also carry reviewable waivers. Active waivers are visible in the
report but do not block the check; expired waivers remain blocking. Waivers
that match no current finding are reported as unused for cleanup.

Existing violations can be captured in a `paddock.baseline/v1` snapshot. A
baseline is tied to the source module and canonical policy SHA-256, uses stable
finding identities, and does not hide new findings. Formatting-only changes do
not require regeneration; semantic policy changes do. Stale entries are
reported so the snapshot can be cleaned up as the architecture improves.
The baseline contract is defined in
[`spec/paddock.baseline-v1.schema.json`](spec/paddock.baseline-v1.schema.json).
Baseline generation also accepts `--graph` or `--adapter`, so external-language
projects can adopt the same “no new violations” workflow.

The service fixtures are in [`examples/services/`](examples/services/README.md).
The acceptance suite runs every good and violating subject through the CLI,
including a deliberately cyclic Go subject, and verifies invalid-policy
failures and the machine-readable report shape.

The real-project translation benchmark is
[`examples/heyrian-platform-subset.yaml`](examples/heyrian-platform-subset.yaml),
with its scope and semantic comparison notes in
[`examples/heyrian-platform-subset.md`](examples/heyrian-platform-subset.md).

Run the focused suite with:

```sh
GOCACHE=.cache/paddock-go-build go test ./paddock/...
```

The current adapters support Go, TypeScript/JavaScript, and Python. All
implement the same adapter registry contract and expose capabilities for source
units and edge kinds. The policy schema is language-neutral, so additional
adapters should produce the same graph model rather than change the rule engine.
On restricted or ephemeral runners, set `GOCACHE` to a writable job-local
directory before checking Go source; failure to access the Go toolchain cache is
an evaluation error with exit code `2`.
External adapter authors can use the machine-readable contracts in
[`spec/`](spec/) and the pass-through conformance fixture in
[`examples/adapter/`](examples/adapter/README.md).
The Python adapter resolves absolute and relative project modules without
importing or executing project code, and classifies standard-library,
third-party, and unresolved imports for policy rules.
The TypeScript adapter resolves relative imports and `tsconfig.json` path
aliases, including local inherited JSONC configs, without requiring npm or a
project build. It also models `.svelte` files and imports from their component
scripts, skips common generated output trees, and can report unresolved imports
explicitly. Policies may optionally set `source.include` and `source.exclude`
to filter relative path patterns; this discovery scope is distinct from
`source.roots`, which scopes policy evaluation. Patterns support `*`, `**`,
and plain directory shorthands.

Rules default to blocking `error` findings, but may declare `warning` or `info`
severity for non-blocking architectural guidance. Required dependencies are
direct by default and can opt into transitive reachability with
`transitive: true`; cycle checks are scoped by source roots and optional rule
selectors.

## Design principle

Paddock should learn the desired architecture from an explicit policy, not infer
it from the existing code and then declare the existing code correct.
