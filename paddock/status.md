# Paddock status

Last updated: 2026-09-15

## Where we left off

Paddock is at a development checkpoint: the core tool is usable as a compiled,
language-neutral architecture gate rather than only as an experimental Go
package. Release publication is intentionally out of scope while development
continues.

The draft-to-seal authoring loop has been exercised with the built-in Go,
TypeScript/JavaScript, and Python adapters, plus an external adapter. The
current work is therefore usage-driven hardening: observe adoption friction,
false positives, adapter gaps, and CI ergonomics before adding more core
surface area.

The portable gate has now been dogfooded against the sibling Overwatch
project without changing Overwatch or publishing anything:

- the locked UI proposal passed with 437 packages, 1,680 edges, and zero
  findings;
- the locked backend review policy returned the expected exit `1` with 40
  findings: 38 `domain-is-pure`, one `application-not-infrastructure`, and one
  `layers-point-inward` finding;
- the Go adapter now keeps `go list` diagnostics on stderr instead of allowing
  cache warnings to corrupt its machine-readable JSON stream. The managed
  workspace still requires a writable Go build cache for the Go tool itself;
  the successful backend run used an isolated temporary cache.
- The backend findings are grouped in
  `examples/overwatch/overwatch-backend-triage.md`; no policy or lock change
  has been made pending architecture-owner decisions.
- An unsealed shared-kernel policy candidate now measures the effect of
  allowing `pkg/id` and `pkg/events`; the current review policy and lock remain
  authoritative. The candidate passed its policy review and reduced the
  backend result from 40 findings to 3, leaving only the `pkg/blob` boundary
  and the `audit/app/query` infrastructure dependency.
- A fresh `init --format json` replay reported 136 source units and 1,607
  edges, but its conservative draft differed from the reviewed backend policy
  in 106 semantic changes. This confirms that initialization is an inventory
  and review starting point, not an automatic policy migration.
- A self-contained proposal fixture now guards the focused authoring loop: a
  shared-kernel approval remains a one-rule diff, passes two expected policy
  cases, and verifies the durable review artifact without creating a lock.
- A second proposal fixture now covers cross-context architecture: adding
  public-API privacy and mediated-access rules remains a two-rule diff, keeps
  the good modular-monolith case passing, and attributes the violating case to
  both intended rules.
- A third proposal fixture now covers hexagonal direction: adding
  `application-points-inward` remains a one-rule diff, keeps the good service
  passing, and attributes the focused adapter violation to that rule alone.
- The TypeScript monorepo now exercises the same proposal loop: adding
  `shared-is-independent` remains a one-rule diff, keeps the good workspace
  passing, and attributes the violating path-alias import to that rule alone.
- The Python adapter now exercises the same loop: adding `domain-is-pure`
  remains a one-rule diff, keeps the good service passing, and attributes a
  domain-to-adapter import without conflating it with cycle detection.
- The external-adapter authoring loop now asserts the machine-readable path:
  an external Rust graph is inspected and mapped, a two-rule proposal is
  reviewed as JSON, the saved review verifies, and the resulting lock-backed
  check passes. This validates the protocol seam without adding Rust logic to
  Paddock.
- A negative external-graph case now proves enforcement as well as plumbing:
  the synthetic adapter can emit a deliberate domain-to-application edge, and
  a one-rule proposal rejects it while its expected-failure review passes.
- A read-only real-world TypeScript/Svelte dogfood run against Heyrian exposed
  and closed two adapter gaps: inherited SvelteKit `$lib` aliases now resolve
  relative to the config that declares them, and `.svelte` files are graph
  source units. Common `.vercel` and `.output` trees are skipped. The resulting
  graph contained 2,876 source units and 8,573 edges; the remaining 1,009
  unresolved edges are concentrated in generated `./$types` imports and
  intentional `?raw` lesson assets, so scan-pattern expressiveness remains an
  ergonomics question rather than another rule kind.
- Heyrian's existing dependency-cruiser check still provides a useful
  comparison boundary: it applies 13 rules to its deliberately scoped 1,956
  modules and 7,924 dependencies with zero violations. The unscoped Paddock
  run scanned more mixed-content source files by design, which confirms that
  adapter discovery scope and policy rule scope need to remain separate and
  explicit.
- The first scan-scope contract is now implemented: optional
  `source.include`/`source.exclude` patterns are validated, included in policy
  diffs and locks, passed to external adapters, and applied to built-in and
  persisted graphs. Replaying Heyrian with a dependency-cruiser-shaped scope
  reduced the Paddock graph to 1,864 source units and 6,980 edges.

The Overwatch backend result is intentional review feedback, not a Paddock
failure. Its policy is not yet an approved compliance gate for that codebase.

## Implemented

### Analysis and enforcement

- Go package graph adapter.
- TypeScript/JavaScript file graph adapter.
- TypeScript adapter support for `.svelte` source files and imports from
  Svelte component scripts.
- TypeScript config resolution relative to each config file in an `extends`
  chain, including generated SvelteKit configs.
- TypeScript exclusion of common `.vercel` and `.output` build trees.
- Adapter-neutral `source.include`/`source.exclude` discovery filters, with
  deterministic path-pattern validation and external-adapter propagation.
- Python file graph adapter.
- External adapter protocol with capability negotiation.
- Adapter conformance validation without policy evaluation.
- Structured adapter validation diagnostics for process, graph, language,
  source-unit, and capability failures.
- `graph --format json` reuses adapter diagnostics for explicit external adapter
  failures.
- Versioned adapter-test manifests and deterministic conformance evidence.
- Adapter-test case results retain stable adapter/assertion error codes.
- Persisted adapter-test results with manifest hash verification.
- Standalone schemas for adapter-test manifests, results, and explanations.
- Adapter-test results can be wrapped as shared `ingen.ci-result/v1` artifacts.
- Portable CI workflow mode for adapter conformance checks.
- Optional shared CI-result envelopes for adapter conformance evidence.
- Component classification and language-neutral policy format.
- Layer direction, allow/deny dependency, cross-context, cycle, coverage,
  required-dependency, component ownership, and unresolved-import rules.
- Warnings, waivers, baselines, and deterministic exit codes.

### Review and CI workflow

- `check`, `graph`, `init`, `baseline`, `ci`, and `explain` commands.
- `init --format json` summaries for agent-facing draft review feedback,
  including source-unit and edge counts.
- Policy tests with expected pass/fail/error cases and required rule IDs.
- Machine-readable policy-test evidence with deterministic finding rule IDs.
- Standalone JSON Schema for `paddock.policy-test-result/v1`.
- Standalone JSON Schema for `paddock.policy-tests/v1` manifests.
- Manifest-only policy-test preflight for agent authoring workflows.
- Policy diff and durable policy review artifacts.
- Policy sealing and lock verification.
- Policy-only validation with normalized JSON output for agent and CI preflight.
- Semantic rule-option validation so required targets/directions are explicit
  and unsupported fields fail before graph analysis.
- JSON diagnostics for invalid policies, including a stable schema and error
  code, while preserving normalized policy JSON for valid input.
- Policy diff and review emit the same diagnostics for invalid before/after
  policies, including the comparison operation and both input paths.
- Validation diagnostics include all independent policy issues with stable
  codes and policy paths.
- Clean JSON reports emit an empty `findings` array rather than `null`, keeping
  the required `paddock.report/v1` collection shape valid for consumers.
- JSON policy-review output contains only the `paddock.policy-review/v1`
  document; human output-path text remains limited to text mode.
- CI result artifacts using `ingen.ci-result/v1`.
- Read-only CI artifact validation for Paddock and external producers.
- Agent-facing text and JSON explanations.
- Standalone JSON Schema for `paddock.explanation/v1` agent handoff artifacts.
- Standalone JSON Schema for `paddock.policy-diff/v1` review evidence.
- Standalone JSON Schema for `paddock.policy-review/v1` durable decisions.
- Standalone JSON Schema for `paddock.policy-lock/v1` sealed policy authority.
- Shared JSON Schema for `ingen.ci-result/v1` CI handoff artifacts.
- Paddock CI artifacts validated against the shared `core/ciresult` envelope.
- Standalone JSON Schema for `paddock.report/v1` dependency findings.
- Standalone JSON Schema for `paddock.baseline/v1` accepted finding identities.
- Agent-facing component dependency maps with grouped internal, external, and
  unresolved edges.
- Portable CI gate support for invoking an external adapter and persisting its
  graph evidence.
- Machine-readable architecture policy schema and documented v1 compatibility
  rules for Paddock contracts.

### Distribution

- Version metadata and `paddock version` text/JSON output.
- Paddock Makefile for build, install, test, vet, release checks, artifact
  generation, and release verification.
- Cross-platform static release archives and SHA-256 checksums.
- `paddock.release/v1` release manifest.
- `paddock release verify` and `paddock.release-verification/v1` results.
- Provider-neutral release checklist and GitHub Actions publishing workflow.

### Dogfood and fixtures

- Go, TypeScript, and Python service examples.
- TypeScript good/violating boundary fixture.
- Dependency-free Python external-adapter conformance fixture.
- Overwatch backend review policy, lock, and policy tests.
- Overwatch UI draft policy, layered proposal, lock, and policy tests.
- Architecture-boundary Go fixtures covering approved shared-kernel and inward
  dependency rules.
- Modular-monolith policy tests covering public APIs, shared kernel, and
  cross-context internal access.
- Cross-language policy tests covering TypeScript feature slices and Python
  hexagonal boundaries.
- TypeScript monorepo fixture covering package-style path aliases across
  workspace packages and shared-to-domain boundary enforcement.
- Initial compiled-binary smoke measurements for the Overwatch backend/UI and
  the TypeScript workspace fixture.
- Draft-to-review authoring-loop coverage proving conservative drafts fail
  policy cases until their architectural boundaries are explicitly reviewed.
- Initialization summaries expose graph scale, unclassified components, and
  warning rules without changing the draft policy itself.
- Layered-direction and non-compiling cyclic Go policy tests.
- GitHub Actions release example and active tag-triggered workflow.

## Important current decisions

- Paddock remains the focus; it is not being folded into Nublar.
- LLMs may propose policies, classify components, and explain findings, but
  they do not decide the authoritative CI verdict.
- Generated draft policies are descriptive until reviewed.
- Sealed policy locks are the CI authority.
- The Overwatch UI proposal intentionally uses one broad `components/**`
  presentation boundary; this is the main vocabulary decision to revisit if
  real UI ownership requires more granularity.

## Next steps

### Should happen next

1. Use the development decision log to keep unresolved policy choices explicit
   and prevent premature expansion of the rule language.
2. Re-run the Heyrian comparison with a real translated policy and record
   path-resolution, performance, false-positive, and policy-authoring friction
   now that discovery scope is explicit.
3. Use that evidence to decide whether the next work is policy vocabulary,
   adapter resolution, or documentation. Overwatch architecture-owner decisions
   remain valuable input, but are not required for Paddock development to
   continue.

### Likely near-term work

- Decide whether the Overwatch backend findings should be fixed, waived, or
  temporarily baselined.
- Decide whether the Overwatch UI component vocabulary needs finer-grained
  presentation roles.
- Exercise the current compatibility expectations in `COMPATIBILITY.md` with
  real adapter and policy changes before freezing more contracts.
- Extend the component map only when real users need additional aggregation or
  visualization detail.

### Defer until real usage asks for them

- Additional language adapters.
- More architecture rule kinds.
- Deeper LLM-specific features.
- Hosted dashboards or centralized policy services.
- Large-scale performance work beyond the first real monorepo measurements.

## Recommended development checkpoint

This checkpoint has now been reached: the draft-to-seal loop, one external
adapter, one real TypeScript/Svelte repository, and an explicit discovery-scope
contract have been exercised. The next stopping point should be after the
scoped repository has been evaluated with a translated real policy. Release
publication remains deferred while development continues.

## Useful resume commands

```sh
GOCACHE=/private/tmp/paddock-go-cache go test ./paddock/...
GOCACHE=/private/tmp/paddock-go-cache go vet ./paddock/...

go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/hexagonal.yaml \
  --cases paddock/examples/hexagonal.policy-tests.yaml
```
