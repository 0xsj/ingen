# Paddock policy design

Status: working design

Paddock policies describe how source code may be composed. They are not runtime
behavioral contracts and must not be confused with Sorna contracts.

## The model

```text
selectors -> components -> graph edges -> rules -> findings
```

### Selectors

A selector maps files or packages to a named component. Selectors may capture
variables from paths:

```yaml
components:
  domain:
    match: internal/{context}/domain/**
    labels:
      role: domain
      context: "{context}"
```

The `{context}` capture allows one rule to apply to every bounded context while
still distinguishing `orders` from `billing`.

Every checked source unit should match exactly one component, unless the policy
explicitly allows an overlap. Unmatched source should be a finding, not an
implicit exemption.

### Graph edges

The first implementation should analyze source imports. The graph should leave
room for later edge kinds:

```text
import       normal source dependency
type-import  compile-time or type-only dependency
test         test-only dependency
generated    generated-source dependency
http-client  client to public API relationship
event        producer/consumer to event contract
schema       code to protobuf/OpenAPI/schema artifact
```

The Go adapter invokes `go list` for build-aware package resolution and parses
import declarations for source locations. The TypeScript adapter walks
TypeScript/JavaScript source files and resolves relative imports without
requiring npm or a project build. The Python adapter walks `.py` files and
resolves absolute and relative module imports without importing or executing
project code. Other languages should enter through the adapter interface and
registry, declare their supported source units and edge kinds, and produce the
same graph shape.

The graph inspection command exposes this adapter boundary:

```sh
paddock graph . --policy paddock.yaml --format json
paddock graph . --language go
paddock graph . --policy paddock.yaml \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace --adapter-arg . \
  --format json > paddock-graph.json
```

The `paddock.graph/v1` result is normalized before output so package and edge
ordering is stable across runs. Classification and rule evaluation happen
after graph loading and remain independent of the adapter implementation.
External process invocation and the `paddock.graph-request/v1` negotiation
contract are specified in [`ADAPTER-PROTOCOL.md`](ADAPTER-PROTOCOL.md).

### External graph adapters

An adapter implemented in another tool or language may emit a validated
`paddock.graph/v1` document and pass it to the policy engine:

```sh
my-rust-adapter --root . > paddock-graph.json
paddock check . --policy paddock.yaml --graph paddock-graph.json
paddock ci . --policy-lock paddock.lock.json --graph paddock-graph.json \
  --output paddock-ci-result.json
```

The document declares its language and source unit, and Paddock rejects a
graph that does not match the policy. The graph hash is recorded in CI results
so an external adapter's exact input is part of the evidence. This is an
interchange boundary, not a way to bypass graph validation or policy rules.

`paddock init` builds a draft policy from that graph. Its component matches are
directory-based guesses and its template rules are emitted with `warning`
severity. The generated file must be reviewed, renamed, and promoted to
blocking severities before it becomes a CI policy; initialization never treats
the existing dependency graph as proof that the proposed architecture is
correct. Generic layered and cyclic drafts may be generated for external
languages; language-specific templates remain adapter-aware.

The source unit is explicit: Go currently uses `package`, while
TypeScript/JavaScript and Python use `file`. Policies may omit it for backward
compatibility, in which case the adapter default is selected from the language.
The TypeScript adapter also honors `compilerOptions.baseUrl` and `paths` from
`tsconfig.json`, including local `extends` chains. It accepts the comments and
trailing commas commonly used in JSONC TypeScript configuration files.
The Python adapter treats `.py` files as source units, resolves absolute and
relative project modules without executing code, and reports project imports
that cannot be resolved as `unresolved` edges.

### Rules

The initial rule vocabulary should cover:

- `allow-dependencies`: a component may depend only on selected targets;
- `deny-dependencies`: a component may not depend on selected targets;
- `layer-direction`: dependencies must move toward a declared layer;
- `no-cross-context`: components with different context labels may not connect;
- `mediated-dependency`: cross-context edges may only target a public API,
  shared kernel, or declared contract;
- `required-dependency`: a component must depend on a declared boundary;
- `component-owns`: selected source units must belong to an approved component;
- `no-cycles`: selected nodes must be acyclic;
- `public-api-only`: external consumers may not reach internal implementation;
- `coverage`: every source unit must be classified;
- `unresolved`: unresolved dependencies are errors or explicit warnings.

Rules should identify the edge that violated them and include a reason that
helps a developer choose an allowed alternative. Every rule has a `severity`:
`error` (the default) blocks the check, while `warning` and `info` findings are
reported but do not block it.

Most dependency rules constrain direct import edges. A
`required-dependency` rule is direct by default; set `transitive: true` when the
requirement is that a source unit can reach an approved target through internal
dependencies. Cycle rules use `source.roots` and their optional `from` selectors
to define the nodes included in the cycle check. An empty `from` selector means
all source units under the configured roots.

`component-owns` is a package-level assertion rather than an edge rule. Its
`allow` values are component names, or label selectors, and its optional `from`
selector narrows which classified packages are checked. This is useful when a
bounded context or source root may contain only a declared set of components;
it also catches an unclassified package in that selected set.

Policy validation also checks rule semantics before graph analysis starts. A
rule must provide the option it needs (`allow`, `deny`, `allow-to`, or
`direction`), and options that the rule kind does not interpret are rejected.
This prevents a typo such as adding `to` to an `allow-dependencies` rule from
silently changing the intended boundary.

### Exceptions

Temporary violations need an explicit, reviewable waiver:

```yaml
waivers:
  - rule: no-cross-context
    from: internal/orders/domain/legacy.go
    to: internal/billing/api
    reason: migration tracked by ARCH-142
    owner: platform-team
    expires: 2027-01-01
```

`from` may identify either the source package path or the source file path;
`to` identifies the target path and may be omitted for a rule-wide source
waiver. Both support the same `*`, `**`, and `{capture}` path matching used by
component selectors.

An active waiver changes the gate decision but does not remove the underlying
finding. The report retains the rule, location, owner, reason, and expiry date.
An expired waiver remains blocking and is marked as expired in text and JSON
reports. Waiver dates are evaluated in UTC and remain active through the stated
date. Waivers that match no current finding are reported as unused so stale
exceptions can be removed; an unused waiver does not change the gate decision.

### Baselines

Baselines are for intentional existing debt when a team wants to enforce “no
new violations” before it can remove all old ones. They are generated from a
current check:

```sh
paddock baseline . --policy paddock.yaml --output paddock-baseline.json
paddock check . --policy paddock.yaml --baseline paddock-baseline.json
paddock baseline . --policy paddock.yaml \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace --adapter-arg . \
  --output paddock-baseline.json
```

The `paddock.baseline/v1` artifact stores stable finding identities based on
rule, kind, source package, target, and source file. Line-number changes do not
invalidate an entry. Baseline findings remain in the report and are marked as
non-blocking; findings not present in the snapshot still fail the check. The
source module and canonical policy SHA-256 must match, and stale snapshot
entries are reported for cleanup. Formatting-only changes do not require
regeneration; semantic policy changes do. Waived findings are not added to a
generated baseline.

### Explanations

Reports carry the rule summaries and component context needed by a separate
explanation step:

```sh
paddock check . --policy paddock.yaml --format json > paddock-report.json
paddock explain paddock-report.json
# Or pass the durable CI envelope directly.
paddock explain paddock-ci-result.json
```

The explanation artifact uses `paddock.explanation/v1`. It describes the
observed edge, the matched constraint, the policy's reason, the finding's
waiver or baseline status, and deterministic remediation suggestions. It is an
agent-facing interpretation of evidence, not a second decision engine. Its
summary groups findings by rule and reports total, active, blocking, waived,
and baselined counts. It also provides a deterministic triage outcome:
`remediate`, `review`, `accepted`, or `clear`. This is an interpretation of
the selected evidence, not a replacement for the complete report verdict.

## Tentative common shape

```yaml
schema: paddock.architecture/v1
project: example
source:
  language: go
  unit: package
  roots: [internal, cmd]

components: {}
rules: []
waivers: []
```

The schema supports sealing so a policy can be referenced from CI, Sentinel,
and evidence without relying on an unchecked mutable file path. Canonicalization
is used for semantic identity; CI artifacts continue to retain the raw file hash
as an exact input reference.

### Sealing

Paddock now supports an explicit policy lock artifact:

```sh
paddock policy seal \
  --input paddock.yaml \
  --output paddock.lock.json

paddock policy verify \
  --policy paddock.yaml \
  --lock paddock.lock.json
```

The `paddock.policy-lock/v1` artifact records the exact source-file SHA-256,
the canonical semantic SHA-256, and the canonical policy payload. `check` and
`ci` can receive `--policy-lock`; with `--policy` present they will refuse to
evaluate a policy whose source or canonical content differs from the lock. This
deliberately makes a comment or formatting change a seal change; policy edits
therefore follow the review sequence of diff, human approval, and re-seal.

When `--policy-lock` is supplied without `--policy`, the canonical policy
embedded in the lock is the evaluation authority. This allows CI to run from
the lock artifact alone while retaining the original policy path and source
hash as provenance.

## LLM boundary

The LLM may:

- translate architectural prose into a proposed policy;
- suggest component classifications;
- explain a violation and possible refactorings;
- propose a time-bounded waiver;
- summarize graph changes in a pull request.

An agent proposing a policy should write a candidate file and run:

```sh
paddock policy diff \
  --before paddock.yaml \
  --after proposed-paddock.yaml \
  --cases paddock-policy-tests.yaml \
  --format json
```

The diff is deterministic, validates both inputs, and records both raw and
canonical hashes for review. When `--cases` is supplied, it also evaluates the
proposed policy against the expected fixture outcomes and embeds the results in
the diff. For an external language, pass `--adapter` and repeated
`--adapter-arg` options; Paddock invokes that adapter once per case root. A
human must approve the proposed policy before it replaces the sealed CI policy.
After approval, the new policy should be sealed and passed to CI with
`--policy-lock`.

For durable review evidence, use:

```sh
paddock policy review \
  --before paddock.yaml \
  --after proposed-paddock.yaml \
  --cases paddock-policy-tests.yaml \
  --output paddock-policy-review.json
```

This writes `paddock.policy-review/v1`, bundling the policy diff and fixture
test results without making the agent the source of the verdict.

The saved artifact can be schema-checked before another system consumes it:

```sh
paddock policy review verify --input paddock-policy-review.json
```

Add `--files` when the original policy and manifest files are available; this
also checks their recorded SHA-256 values and detects post-review edits.

The LLM must not silently alter a sealed policy or produce the authoritative CI
verdict. Proposed changes should appear as a policy diff for human review.

## Open design questions

- Should selectors classify files, packages, modules, or all three?
- Are type-only imports architecture edges by default?
- How should generated code be classified?
- Should layer order use numbers, names, or a partial order?
- Which cross-language relationships can be represented without weakening
  language-specific analysis?
- Is the first policy format YAML, JSON, or both?
