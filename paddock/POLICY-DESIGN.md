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

Go analysis should use Go's package and type information rather than regular
expressions. Other languages should enter through adapters that produce the
same graph shape.

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

A waiver changes the gate decision; it does not remove the underlying finding.
Expired waivers must fail or become a clearly visible release blocker.

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

