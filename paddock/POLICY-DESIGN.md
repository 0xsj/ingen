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

Go analysis should use the Go toolchain's package information and parser rather
than regular expressions. The first adapter invokes `go list` for build-aware
package resolution and parses import declarations for source locations. Other
languages should enter through adapters that produce the same graph shape.

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
helps a developer choose an allowed alternative.

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
date.

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
source module must match, and stale snapshot entries are reported for cleanup.
Waived findings are not added to a generated baseline.

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
