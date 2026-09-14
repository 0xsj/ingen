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
requiring npm or a project build. Other languages should enter through the
adapter interface and registry, declare their supported source units and edge
kinds, and produce the same graph shape.

The graph inspection command exposes this adapter boundary:

```sh
paddock graph . --policy paddock.yaml --format json
paddock graph . --language go
```

The `paddock.graph/v1` result is normalized before output so package and edge
ordering is stable across runs. Classification and rule evaluation happen
after graph loading and remain independent of the adapter implementation.

The source unit is explicit: Go currently uses `package` and
TypeScript/JavaScript currently uses `file`. Policies may omit it for backward
compatibility, in which case the adapter default is selected from the language.
The TypeScript adapter also honors `compilerOptions.baseUrl` and `paths` from
`tsconfig.json`, including local `extends` chains. It accepts the comments and
trailing commas commonly used in JSONC TypeScript configuration files.

### Rules

The initial rule vocabulary should cover:

- `allow-dependencies`: a component may depend only on selected targets;
- `deny-dependencies`: a component may not depend on selected targets;
- `layer-direction`: dependencies must move toward a declared layer;
- `no-cross-context`: components with different context labels may not connect;
- `mediated-dependency`: cross-context edges may only target a public API,
  shared kernel, or declared contract;
- `required-dependency`: a component must depend on a declared boundary;
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
```

The `paddock.baseline/v1` artifact stores stable finding identities based on
rule, kind, source package, target, and source file. Line-number changes do not
invalidate an entry. Baseline findings remain in the report and are marked as
non-blocking; findings not present in the snapshot still fail the check. The
source module and exact policy-file SHA-256 must match, and stale snapshot
entries are reported for cleanup. If the policy changes, regenerate the
baseline. Waived findings are not added to a generated baseline.

### Explanations

Reports carry the rule summaries and component context needed by a separate
explanation step:

```sh
paddock check . --policy paddock.yaml --format json > paddock-report.json
paddock explain paddock-report.json
```

The explanation artifact uses `paddock.explanation/v1`. It describes the
observed edge, the matched constraint, the policy's reason, the finding's
waiver or baseline status, and deterministic remediation suggestions. It is an
agent-facing interpretation of evidence, not a second decision engine.

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

The schema should later gain canonicalization and sealing so a policy can be
referenced from CI, Sentinel, and evidence without relying on a mutable file
path.

## LLM boundary

The LLM may:

- translate architectural prose into a proposed policy;
- suggest component classifications;
- explain a violation and possible refactorings;
- propose a time-bounded waiver;
- summarize graph changes in a pull request.

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
