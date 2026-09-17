# Paddock roadmap

Last updated: 2026-09-17

Paddock is InGen's focused architecture-verification tool. It turns source
graphs and explicit architecture decisions into deterministic, reviewable
checks that can run locally, in CI, or as a tool used by a coding agent.

The roadmap is intentionally development-oriented. Release publication,
hosted services, and broad language coverage remain secondary to proving that
the current workflow is useful and trustworthy on real projects.

## Current position

Paddock is at the usable development-checkpoint stage. The core loop works:

```text
discover -> classify -> build graph -> evaluate policy -> explain evidence
                                      -> review and seal approved changes
```

The authoritative verdict remains deterministic. An LLM may use Paddock's
structured tools to inspect a system, propose classifications or policies,
explain findings, and prepare a review. It must not silently approve a policy,
change a sealed lock, or decide the CI result.

## Completed milestones

### Foundation and policy engine

- Created the standalone Paddock Go CLI and internal package structure.
- Added language-neutral policies with component classification and selectors.
- Implemented rules for layer direction, allow/deny dependencies, cycles,
  cross-context access, mediated dependencies, public APIs, coverage, required
  dependencies, component ownership, and unresolved imports.
- Added warnings, waivers, baselines, deterministic findings, and exit codes.
- Added architecture examples for layered, hexagonal, clean/onion-style,
  modular-monolith, vertical-slice, feature-sliced, and migration boundaries.

### Language and adapter boundary

- Added built-in Go package, TypeScript/JavaScript file, and Python file
  adapters.
- Added `.svelte` source handling, inherited SvelteKit alias resolution, and
  common generated-tree exclusions for the TypeScript adapter.
- Defined the external adapter request/response protocol with capabilities.
- Added adapter validation diagnostics and conformance manifests/results.
- Added reusable adapter profiles with portable executable and argument
  resolution, profile validation, exact-file SHA-256 verification, and profile
  provenance.
- Added profile-backed adapter conformance tests so architecture gates and
  adapter tests can use the same reviewed configuration.

### Review, CI, and agent handoff

- Added `init`, `map`, `graph`, `check`, `baseline`, `ci`, and `explain` flows.
- Added policy diff, policy review, policy sealing, and lock verification.
- Added policy-test manifests with expected pass/fail/error cases.
- Added durable graph, report, explanation, review, lock, baseline, release,
  adapter, and shared CI-result schemas.
- Added provider-neutral CI workflow and handoff behavior that preserves the
  authoritative exit code while emitting agent-facing explanations.
- Added exact input hashes for policy, locks, graphs, baselines, manifests,
  and adapter profiles in durable CI evidence.

### Real-project and comparison evidence

- Dogfooded the gate against Overwatch without changing its policies or source.
  The UI policy passes; the backend policy intentionally reports 40 findings
  pending architecture-owner decisions.
- Replayed the handoff path against both passing and failing Overwatch cases.
- Compared a translated 13-rule Heyrian platform subset with its existing
  dependency-cruiser check and achieved source-path parity for the relevant
  TypeScript/Svelte inputs.
- Added focused fixtures for scan scope, path selectors, internal targets,
  external package families, configuration portability, and negative adapter
  enforcement.
- Current checked-in state is `v0.9.51`; the full Paddock/Core test and vet
  suites pass.

## Next phase: adoption-driven validation

This is the immediate priority. Use Paddock as an actual agent tool on a real
repository handoff and record the first concrete friction instead of adding
another speculative rule kind.

### Work items

1. Run the documented Overwatch backend and UI handoff flows with a coding
   agent consuming the JSON explanation and CI artifact.
2. Observe whether the agent has enough context to identify the affected
   boundary, understand the relevant rule, and propose a safe next action.
3. Record any problem as one of:
   - missing context or provenance;
   - noisy, duplicated, or poorly prioritized findings;
   - unclear remediation guidance;
   - adapter setup, path, cache, or workspace friction;
   - policy authoring or review friction.
4. Choose one concrete issue, reproduce it in a focused fixture, fix it, and
   add a regression test before changing broader contracts.

### Exit criteria

- At least one real agent handoff has been consumed end to end.
- The first observed issue is either fixed with a regression test or recorded
  as an explicit product decision.
- Existing Go, TypeScript, Python, and external-adapter acceptance flows still
  pass.
- No architecture policy, waiver, baseline, or lock is changed without an
  explicit owner decision.

### Current slice result

The 2026-09-17 Overwatch handoff and compatibility replay satisfied this
checkpoint. The backend explanation was actionable, the UI explanation was
clear, both sealed locks verified, both generated CI envelopes validated as
independent consumer inputs, and the UI draft-to-proposal review passed all
three cases with 22 normalized changes. No concrete Paddock defect was
observed in those handoffs. The performance replay then exposed a concrete
provenance gap caused by dirty sibling worktrees changing graph counts between
runs; optional source VCS identity and a changes-only hash were added to the CI
envelope and filtered explanation provenance with compatibility coverage.
Paddock producer version is now carried alongside that identity so results from
different Paddock builds can also be distinguished. The next implementation
should begin only when real agent usage exposes another
specific false positive, missing context, adapter/setup friction, or
policy-authoring problem.
A negative replay of an unusable Go cache returned exit `2` with a valid error
artifact and the underlying `go list` diagnostic, so runner-owned cache setup
remains sufficient for now and a Paddock-controlled cache stays deferred.
The next discovery replay found that component maps collapsed templated
component names across bounded contexts; additive component and dependency
identities now preserve those label variants without changing base names.
The following init replay exposed a related authoring gap: graph counts could
be compared across dirty worktrees without proving that they came from the
same source or normalized graph. `paddock.init/v1` now carries optional source
VCS identity and a graph SHA-256 for that correlation.
Two consecutive dirty-Overwatch backend init runs then produced identical
normalized summaries after excluding only their destination paths, so no new
provenance defect was found. Continue with real agent consumption before
expanding the init contract further.
An end-to-end temporary Git fixture also verifies that a source change updates
both `changes_sha256` and `graph_sha256` while preserving the committed
revision.
The next backend/UI handoff demonstrated the value of that identity: the
backend stayed at 162/1,780 source units/edges, while the dirty UI moved from
451/1,776 during init to 453/1,791 at handoff. The UI revision was unchanged
but its changes hash differed, correctly marking the earlier summary stale;
the current backend explanation stayed actionable and the UI gate stayed
passing.

## Near-term hardening

After the adoption pass, prioritize only issues supported by evidence:

- Improve explanation context or triage when an agent cannot act on a finding.
- Add stable adapter diagnostic codes only for failures observed in practice.
- Measure graph construction and policy evaluation on a larger real repository.
- Revisit path/workspace and symlink behavior if real monorepos expose gaps.
- Revisit generated code and type-only imports when an adapter demonstrates a
  meaningful false positive.
- Revisit multiple independent TypeScript project configs when a real target
  requires project references or package-local configs.
- Extend component maps only when users need a missing aggregation or
  visualization detail.

Each hardening change should include a small fixture, a contract decision when
needed, machine-readable evidence, and an updated compatibility note.

## Later expansion, only when requested by usage

- Additional language adapters such as Rust, Java, Kotlin, or C# as external
  adapters first, then built-ins only if repeated usage justifies ownership.
- Cross-language boundaries for concrete APIs, events, schemas, or generated
  clients.
- More rule kinds or richer partial-order layer semantics.
- Performance and incremental graph work after real measurements identify a
  bottleneck.
- Optional JSON policy input after a consumer needs it.

## Explicitly deferred

- Hosted dashboards or centralized policy services.
- Paddock-specific LLM decision-making or autonomous policy approval.
- Automatic policy migration from `init` output.
- Default global exemptions for generated code.
- A Paddock-controlled Go cache before documented cache setup proves
  insufficient.
- Treating the Heyrian translation as a full dependency-cruiser replacement;
  the comparison remains a semantic regression benchmark.
- Folding Paddock into Nublar. Paddock remains a focused standalone tool in
  the wider InGen ecosystem.

## Development stopping points

Pause and review the roadmap when any of these conditions is met:

- The real agent handoff completes without a concrete usability issue.
- A proposed change would add a rule kind without a real failing example.
- A proposed adapter would be used only by a synthetic fixture.
- A contract change cannot be justified by an observed consumer need.
- A policy, waiver, baseline, or lock change would require architecture-owner
  approval rather than Paddock implementation work.

At those points, preserve the current regression suite and collect usage data
before expanding the surface area.

## References

- [Status](status.md)
- [Development decisions](DECISIONS.md)
- [Compatibility policy](COMPATIBILITY.md)
- [Agent guide](AGENTS.md)
- [Adapter protocol](ADAPTER-PROTOCOL.md)
- [Command reference](README.md)
