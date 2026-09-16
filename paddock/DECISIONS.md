# Paddock development decisions

Last updated: 2026-09-16

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
- The Python adapter completes the current built-in adapter matrix: a focused
  domain-to-adapter import is accepted by the before-policy and rejected by a
  one-rule `domain-is-pure` proposal, while the good service remains passing
  and no unrelated cycle finding is introduced.
- The existing external-adapter authoring loop now protects the agent-facing
  path as well as protocol conformance: a synthetic Rust graph can be mapped,
  reviewed as JSON, verified by file hashes, sealed, and checked through the
  lock. This is evidence that external languages do not need policy-engine
  changes, not evidence that Paddock should own their parsers.
- The external seam also has a negative enforcement case: the synthetic
  adapter can emit a deliberate domain-to-application edge, and a focused
  one-rule proposal reports `domain-is-pure` while the expected-failure review
  remains a passing review artifact.
- A read-only run against the real Heyrian TypeScript/Svelte repository found
  2,876 source units and 8,573 edges after the adapter learned `.svelte`
  sources, inherited SvelteKit `$lib` aliases, and common `.vercel`/`.output`
  exclusions. The run still has 1,009 unresolved edges, mostly generated
  `./$types` imports and intentional `?raw` lesson assets. This is concrete
  evidence that source discovery scope is a product concern for mixed-content
  repositories.
- Heyrian's existing dependency-cruiser check is a useful external comparison:
  its deliberately scoped check applies 13 rules to 1,956 modules and 7,924
  dependencies and reports zero violations. The unscoped Paddock run
  discovered the broader source tree; Heyrian's check excludes lesson examples
  and runs only over its selected platform inputs.
- The first scan-scope contract is now implemented. Optional
  `source.include`/`source.exclude` patterns are validated and canonicalized,
  participate in policy diffs and locks, reach external adapter requests, and
  filter persisted graphs as well as built-in adapter graphs. A Heyrian-shaped
  scope reduced Paddock's graph from 2,876 source units/8,573 edges to 1,864
  source units/6,980 edges.
- A translated Heyrian platform subset now passes ten representative Paddock
  rules with zero findings. The complete comparison had 2,175 in-repository
  TypeScript/Svelte source units in both tools. Their total graph
  counts intentionally differ: dependency-cruiser exposes external and asset
  modules, while Paddock represents those as edge target kinds. This is
  sufficient evidence for a semantic rule comparison, not a claim of identical
  graph accounting.
- The latest Heyrian replay is complete again. Paddock's 13-rule subset passes
  with 2,209 source units and 8,215 edges; dependency-cruiser reports 2,302
  total modules, 9,382 dependencies, and zero violations across 13 rules. Both
  tools enumerate the same 2,209 in-repository TypeScript/Svelte source paths.
  No target-project changes were made to produce this result.

## Translation boundary from this cycle

- Keep `examples/heyrian-platform-subset.yaml` as a real-policy benchmark, but
  do not present it as a full dependency-cruiser migration. It covers kernel,
  HTTP, services, root, and platform-cycle constraints that map directly to
  current components and rules.
- Use reserved `path`/`path-not` rule selectors for path predicates rather than
  forcing every source unit into a positive client/server component split.
  `|`-joined alternatives cover the compound server-only predicate observed in
  Heyrian while leaving component classification positive and reviewable.
- Keep external package-family ownership at the edge level. The current graph
  retains the raw external import name, and `external: "@vendor/*"` matches
  package families without expanding vendor code. Do not add richer package
  metadata unless a real adapter needs it.
- Use internal target `path` patterns for concrete adapter and portability
  boundaries. This keeps repository-specific folder ownership expressible
  without introducing a new rule kind or over-classifying every source unit.
- Keep explanation remediation deterministic and grounded in the normalized
  rule summary. Deny-dependency suggestions now repeat the denied targets so an
  agent can see the boundary constraint without reparsing the policy file.
- Preserve duplicate rule signals but relate them by source, target, and source
  location. This lets an agent collapse repeated symptoms around one edge while
  keeping every rule's independent attribution available for review.
- Treat the explanation → policy-review sequence as the agent handoff boundary:
  Paddock may narrow evidence and measure a proposal, but proposal acceptance
  remains an explicit architecture-owner decision.
- Keep CI orchestration provider-neutral. The example `handoff` helper may
  compose gate and explanation artifacts, but it must preserve the gate exit
  code and never turn explanation output into a second verdict.

## Scope decision from this cycle

- Do not add multi-project TypeScript config discovery yet. Reopen it when a
  real target has independent package configs or project references that the
  root-config model cannot represent.
- Keep scan scope separate from `source.roots`: roots determine which
  discovered units rules evaluate, while `source.include`/`source.exclude`
  determine which files become graph evidence at all. The initial contract is
  intentionally path-based and adapter-neutral; richer adapter-native config
  should wait for a concrete project need.
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
- Whether path and workspace resolution behave predictably in monorepos and
  symlinked directories; inherited SvelteKit config and common generated build
  trees now have a real regression check.
- Whether path-based scan patterns are expressive enough for real projects,
  especially when a repository's architecture tool scopes by entrypoint globs
  rather than directory boundaries.
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

The Heyrian translation cluster is complete for the current 13-rule config:
adapter-folder ownership, memory-adapter selection, and configuration
portability are all represented and pass the real-project replay. Keep the
Heyrian source-path parity check and external-package fixture as regression
benchmarks. The next batch should be driven by a concrete false positive,
adapter metadata need, or authoring ergonomics gap rather than another
speculative rule kind.
