# Paddock status

Last updated: 2026-09-16

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
- A fresh locked Overwatch report now exercises the agent-facing explanation
  path: 40 findings are grouped into three rules, triage is `remediate`, and
  deny/allow remediation suggestions include the concrete boundary targets.
  The explanation remains advisory and does not alter the locked verdict.
- The full handoff loop is now replayed: a filtered explanation isolates the
  application/infrastructure edge, and the unsealed shared-kernel proposal
  produces exactly two policy changes with a passing expected-review case and
  three remaining findings. Paddock still does not approve or apply it.
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
- A translated Heyrian platform subset now lives in
  `examples/heyrian-platform-subset.yaml`. Against the current workspace it
  passes ten representative rules with zero findings. The complete comparison
  showed Paddock and dependency-cruiser enumerating the
  same 2,140 in-repository `.ts`/`.svelte` source units; their total module and
  edge counts are not directly equivalent because dependency-cruiser
  materializes external and asset modules while Paddock keeps those as edge
  target kinds. The comparison is therefore about rule findings and source-
  path coverage, not identical graph totals.
- The latest Heyrian replay is parseable again. Paddock's 13-rule subset passes
  with 2,209 source units, 8,215 edges, and zero findings; dependency-cruiser
  passes with 2,302 total modules, 9,382 dependencies, 13 rules, and zero
  violations. The two tools enumerate the same 2,209 in-repository `.ts` and
  `.svelte` source paths. No Heyrian files were changed by this work.
- The translation now covers the server-only predicate as well: rule selectors
  support `path`/`path-not` with `|`-joined alternatives for server directories,
  route suffixes, and special files. External package-family matching was
  handled separately with a focused fixture and is supported at the edge level.
- Internal target path patterns are now supported. A focused TypeScript fixture
  and the Heyrian policy exercise adapter-folder ownership and memory-adapter
  selection, and configuration portability without adding another rule kind.
- A focused configuration-portability fixture now proves that the same rule
  catches internal server/UI targets, `$app`/`$env` imports, and Svelte package
  families without relying only on a clean real-project replay.

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
- Reusable `paddock.adapter-profile/v1` files with per-root argument
  expansion for direct commands and policy-test workflows.
- `adapter profile validate` preflight diagnostics that check profile shape and
  executable availability without launching the adapter.
- `adapter profile verify` exact-file SHA-256 checks for reviewed profiles,
  including optional enforcement in the portable CI helper.
- Profile-backed graph evidence records the profile path and SHA-256 in CI and
  agent-facing explanation provenance.
- Profile-backed CI results now retain the exact profile as the shared
  `adapter_profile` input reference.
- Adapter conformance validation without policy evaluation.
- Structured adapter validation diagnostics for process, graph, language,
  source-unit, and capability failures.
- `graph --format json` reuses adapter diagnostics for explicit external adapter
  failures.
- Versioned adapter-test manifests and deterministic conformance evidence.
- Adapter-test manifests can reuse a profile, with profile hashes retained in
  conformance results and their shared CI envelopes.
- Adapter-test case results retain stable adapter/assertion error codes.
- Persisted adapter-test results with manifest hash verification.
- Standalone schemas for adapter-test manifests, results, and explanations.
- Adapter-test results can be wrapped as shared `ingen.ci-result/v1` artifacts.
- Portable CI workflow mode for adapter conformance checks.
- Optional shared CI-result envelopes for adapter conformance evidence.
- The portable workflow now has a real Rust example that runs adapter
  conformance, policy sealing/verification, and both passing and failing
  external-adapter gates with durable graph and CI-result validation.
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
- Deny-rule explanations include the normalized denied targets in their
  remediation suggestions, keeping internal path and external package-family
  constraints visible to text-only agents and reviewers.
- Explanation findings now identify related rule IDs when multiple rules report
  the same source-to-target edge. Overwatch's application/infrastructure edge
  is the first real dogfood case; both rule signals remain preserved.
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
- Provider-neutral CI `handoff` mode now runs the lock-backed gate and emits a
  filtered explanation as a separate agent-facing artifact while preserving
  the original gate exit code.
- The handoff helper was replayed against both real Overwatch gates: the
  backend returned `1` with a filtered failing explanation, while the UI
  returned `0` with a clear explanation; stdout stayed machine-readable in both
  cases.
- The external-adapter acceptance workflow now covers `handoff` as well as
  gate/review/seal, confirming that adapter-produced graph evidence and the
  explanation artifact travel through the same provider-neutral seam.
- The portable CI acceptance workflow now covers a built-in Go failing handoff
  under the helper's `set -e` shell mode, preserving exit `1`, the failed CI
  artifact, and the filtered explanation without mixed stdout.
- The same acceptance workflow now exercises an intentionally failing external
  graph: `handoff` preserves exit `1`, retains the one finding in the CI
  artifact, and emits a filtered `domain-is-pure` explanation with no mixed
  stdout.
- A fresh compatibility replay of the real Overwatch backend proposal keeps
  the sealed lock valid, reports exactly two normalized policy changes, and
  verifies a durable `PASS` review artifact with one expected-failure case and
  three measured findings. Neither the sealed policy nor Overwatch was changed.
- Repository-local `AGENTS.md` guidance now defines the safe agent workflow,
  result interpretation, adapter-failure handling, expected exit-`1` handling
  in `set -e` scripts, and the human approval boundary for policy changes.
- CI-derived explanations now retain optional artifact, policy, lock, graph,
  and baseline provenance so filtered agent handoffs can be correlated with
  their authoritative evidence; acceptance tests independently recompute the
  artifact hash and compare the lock reference.
- External graph evidence now records non-secret adapter invocation metadata,
  and CI-derived explanations surface it when the retained graph is available;
  raw adapter arguments remain excluded from artifacts.
- The real Overwatch handoff was replayed in a restricted runner with an
  explicit writable `GOCACHE`: the backend returned the expected blocking
  result and the UI returned a clean result. The setup requirement is now
  documented as an adapter evaluation prerequisite; cache failures remain
  exit-`2` errors.
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
- Dependency-free Python AST adapter example that discovers and resolves real
  local imports, with good/violating policy checks and CI provenance coverage.
- Dependency-free Rust `use`/`mod` adapter example that proves an unsupported
  language can provide a real file graph, pass a hexagonal policy, and surface
  purity/cycle findings through the same external-adapter seam. It is
  intentionally a constrained parser, not a complete Rust front end. Its
  committed adapter-test manifest covers both graph shapes and rejects a
  non-Rust request.
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
- Heyrian platform-subset translation benchmark covering all 13 rules in the
  current dependency-cruiser config, while documenting the remaining semantic
  translation boundaries.
- External package-family targets now support segment patterns such as
  `external: "@supabase/*"`; a two-file TypeScript fixture proves that an
  unapproved importer is attributed to the ownership rule without installing
  or traversing the vendor package.
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
2. Keep the 13-rule Heyrian subset and its source-path parity replay as a
   regression benchmark while collecting feedback from real agent handoffs.
3. Keep the focused configuration-portability fixture, selector fixtures, and
   external-adapter handoff as negative regressions; the current real-project
   replays are clean where expected.
   Overwatch architecture-owner decisions remain valuable input, but are not
   required for Paddock development to continue.

### Likely near-term work

- Decide whether the Overwatch backend findings should be fixed, waived, or
  temporarily baselined.
- Decide whether the Overwatch UI component vocabulary needs finer-grained
  presentation roles.
- Exercise the current compatibility expectations in `COMPATIBILITY.md` with
  real adapter and policy changes before freezing more contracts.
- Extend the component map only when real users need additional aggregation or
  visualization detail.
- Use the documented Overwatch handoff command with an actual coding agent and
  record any missing context, noisy findings, or awkward environment setup as
  the next adoption-driven issue. Both the blocking backend and passing UI
  replays are now documented and verified.

### Defer until real usage asks for them

- Additional language adapters.
- More architecture rule kinds.
- Deeper LLM-specific features.
- Hosted dashboards or centralized policy services.
- Large-scale performance work beyond the first real monorepo measurements.

## Recommended development checkpoint

This checkpoint has now been reached: the draft-to-seal loop, one external
adapter, one real TypeScript/Svelte repository, an explicit discovery-scope
contract, a translated real-policy subset, path selectors, internal target
paths, and external package-family ownership have been exercised. A durable
complete comparison note is recorded, and the remaining work should now be
driven by real false positives, adapter needs, or authoring friction. Release
publication remains deferred while development continues.

## Next development batch

Keep the 13-rule Heyrian comparison and its source-path parity replay as
regression benchmarks. The agent handoff path is now documented against the
real Overwatch backend; the next implementation batch should address the first
concrete issue observed when an agent consumes that handoff—missing context,
noisy findings, or adapter setup friction—rather than adding speculative rule
kinds.

## Useful resume commands

```sh
GOCACHE=/private/tmp/paddock-go-cache go test ./paddock/...
GOCACHE=/private/tmp/paddock-go-cache go vet ./paddock/...

go run ./paddock/cmd/paddock policy test \
  --policy paddock/examples/hexagonal.yaml \
  --cases paddock/examples/hexagonal.policy-tests.yaml
```
