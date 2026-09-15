# Paddock development decisions

Last updated: 2026-09-15

This is a working decision log for Paddock development. It records product
boundaries and unresolved design choices; it is not a replacement for the
versioned schemas, sealed policy locks, or CI verdicts.

## Decisions to preserve

- Paddock remains a focused architecture-verification tool. It is not being
  folded into Nublar.
- The graph adapter owns language-specific parsing and resolution. The policy
  engine owns classification, rules, findings, and verdicts.
- An LLM may propose policies, classifications, explanations, and waivers, but
  it cannot silently change a sealed policy or produce the authoritative CI
  verdict.
- Draft policies are descriptive until reviewed. A sealed policy lock is the
  CI authority.
- New language adapters and rule kinds should be driven by real usage rather
  than added speculatively.

## Resolved usage finding

- A clean Overwatch UI JSON report initially serialized `findings` as `null`,
  although `paddock.report/v1` requires an array. The report producer now
  initializes the collection to `[]`, and an acceptance test protects the
  machine-readable contract.
- `policy review --format json` initially appended a human output-path line to
  stdout. It now keeps stdout parseable as one `paddock.policy-review/v1`
  document, matching the behavior expected by agents and CI.

## Recent usage evidence

- A TypeScript monorepo-shaped fixture resolves package-style aliases across
  workspace packages and reports the intended shared-to-domain boundary
  findings. This confirms the current root `tsconfig.json` path-alias model;
  it does not yet establish behavior for multiple independent TypeScript
  project configs.
- A compiled development binary checked the 136-package/1,607-edge Overwatch
  backend in about 0.25 seconds and the 437-package/1,680-edge UI in about
  0.06 seconds with a warm isolated cache. These are local smoke measurements,
  not performance benchmarks.
- In the managed workspace, the Go adapter needs a writable `GOCACHE`; without
  it, `go list` can fail before graph analysis. This is documented operational
  setup for now and may become a Paddock-controlled cache option if real users
  encounter the same friction.
- `init --format json` now exposes graph scale and review workload directly.
  Current Overwatch drafts report 136 backend source units with 1,607 edges and
  437 UI source units with 1,680 edges; they surface 38 unclassified backend
  components and 18 UI components. The monorepo fixture reports four source
  units, three edges, and two broad workspace components. These remain review
  prompts, not architecture verdicts. The counts are additive optional fields
  in `paddock.init/v1`, preserving compatibility for older summary consumers.
- Replaying the authoring loop against Overwatch confirms that a conservative
  `init` draft is an inventory starting point, not an automatic migration: its
  comparison with the reviewed backend policy contains 106 semantic changes,
  mostly replacing the established component vocabulary. The reviewed
  shared-kernel proposal remains a focused two-change diff and passes its
  expected-failure review with three remaining findings.
- The focused proposal loop is now protected by the architecture-boundaries
  fixture: the before-policy rejects the shared-kernel value, the candidate is
  exactly one rule change, and its two-case review passes while preserving the
  other boundary findings. This is the minimum authoring behavior to preserve
  before adding richer proposal automation.
- The same loop is now protected for modular-monolith boundaries: the
  before-policy intentionally leaves cross-context access unregulated, while
  the candidate adds exactly `context-internals-are-private` and
  `cross-context-access-is-mediated`. The good case stays passing and the
  violating case must name both rules.
- The proposal loop now also covers hexagonal direction: a small violating
  service imports an outbound adapter from application code, the before-policy
  accepts it, and the one-rule candidate rejects it specifically through
  `application-points-inward`. This gives the current workflow coverage for
  shared-kernel, cross-context, and inward-dependency proposals.
- The TypeScript monorepo confirms that this proposal model is language-neutral:
  the before-policy accepts a shared package importing an orders domain via a
  configured alias, and the one-rule candidate rejects it specifically through
  `shared-is-independent` while preserving the good workspace case.

## Scope decision from this cycle

- Do not add multi-project TypeScript config discovery yet. Reopen it when a
  real target has independent package configs or project references that the
  root-config model cannot represent.
- Do not add a Paddock-controlled Go cache option yet. Reopen it if ordinary
  local or CI environments reproduce the managed-workspace cache failure after
  the documented `GOCACHE` setup is applied.

## Open questions and current posture

### Selector granularity

Paddock currently supports path-based component classification with adapter-
appropriate source units. Keep that model until a real project needs a
distinct module abstraction; adding files, packages, and modules as separate
first-class concepts too early would increase policy complexity.

### Type-only imports

The current graph model is primarily import-oriented and does not yet make
type-only relationships a universal policy decision. Preserve the distinction
as an adapter capability when a language can report it, then decide the default
after a TypeScript or similar project demonstrates a meaningful false positive.

### Generated code

Generated files need an explicit project policy rather than an implicit global
exemption. Before enforcing a default, measure how each adapter identifies
generated sources and whether generated edges should be excluded, reported, or
checked under a separate component.

### Layer ordering

Named layers are easier for humans and LLMs to author; numeric order is easier
to compare mechanically. Keep the current rule representation stable while
real policies are exercised, and only generalize to a partial order if
multiple projects require non-linear layering.

### Cross-language relationships

Paddock can combine language-neutral graph documents through the external
adapter boundary, but it should not weaken language-specific analysis to make
all relationships look identical. Cross-language rules should be added only
after a concrete boundary—such as an API, event, schema, or generated client—
has a fixture and a clear ownership model.

### Policy authoring format

YAML remains the human authoring format for the current examples. JSON remains
the normalized and machine-readable artifact format. Supporting JSON policy
input can wait until a consumer or integration needs it; the v1 contract does
not need two authoring syntaxes yet.

## Usage notes still to collect

- Which adapter failures need additional stable diagnostic codes beyond the
  current process, graph, language, source-unit, and capability categories.
- Whether path and workspace resolution behave predictably in monorepos,
  symlinked directories, and generated build trees.
- First real performance measurements for graph construction and policy
  evaluation on a larger repository.
- Which findings are false positives often enough to justify a rule or selector
  change.
- Whether the draft-to-seal workflow gives an agent enough evidence to propose
  safe changes without adding provider-specific integration.
- Whether conservative `init` output is sufficiently actionable for real
  projects; the monorepo draft currently identifies broad workspace areas but
  leaves domain/application roles for human review by design.

## Next development batch

Use one real target to turn the usage notes above into evidence. Prioritize a
small compatibility or ergonomics fix over adding a new rule kind, unless the
target exposes a clear missing architectural primitive.
