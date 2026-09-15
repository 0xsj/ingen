# Paddock status

Last updated: 2026-09-15

## Where we left off

Paddock has reached a release-candidate checkpoint. The core tool is usable as
a compiled, language-neutral architecture gate rather than only as an
experimental Go package.

The current release candidate exercise was `0.1.0-rc1`:

- four static artifacts were built for macOS/Linux on amd64/arm64;
- the `paddock.release/v1` manifest and SHA-256 checksums were generated;
- the compiled binary independently verified the release bundle;
- the full Paddock test suite and `go vet` passed;
- the Overwatch UI locked-policy gate passed with exit `0`;
- the Overwatch backend locked-policy gate returned the expected exit `1` with
  40 architectural findings.

The backend result is intentional review feedback, not a Paddock failure. The
backend policy is not yet an approved compliance gate for that codebase.

No `paddock-v0.1.0-rc1` tag or hosted GitHub release has been created yet.
The local release and verification path is ready for that external step.

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

## Implemented

### Analysis and enforcement

- Go package graph adapter.
- TypeScript/JavaScript file graph adapter.
- Python file graph adapter.
- External adapter protocol with capability negotiation.
- Adapter conformance validation without policy evaluation.
- Versioned adapter-test manifests and deterministic conformance evidence.
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

1. Obtain the Overwatch architecture-owner decision on the backend triage:
   shared-kernel policy expansion, code fixes, waivers, or temporary baseline.
2. Revisit the UI proposal’s broad `components/**` vocabulary with actual
   ownership feedback.
3. Record adapter, path-resolution, performance, and policy-authoring issues
   from the external run before expanding the core schemas.

### Likely near-term work

- Decide whether the Overwatch backend findings should be fixed, waived, or
  temporarily baselined.
- Decide whether the Overwatch UI component vocabulary needs finer-grained
  presentation roles.
- Freeze and document compatibility expectations for `paddock.graph/v1`, the
  policy format, and the CI/release artifact schemas. (The first version of
  this is now in `COMPATIBILITY.md`.)
- Add release workflow smoke coverage if hosted CI exposes issues not visible
  locally.
- Extend the component map only when real users need additional aggregation or
  visualization detail.

### Defer until real usage asks for them

- Additional language adapters.
- More architecture rule kinds.
- Deeper LLM-specific features.
- Hosted dashboards or centralized policy services.
- Large-scale performance work beyond the first real monorepo measurements.

## Recommended development checkpoint

The next useful stopping point is after the draft-to-seal authoring loop and
one external adapter have been exercised. At that checkpoint, use observed
adoption friction, false positives, adapter gaps, and CI ergonomics to decide
which development work deserves priority. Release publication remains
deferred while development continues.

## Useful resume commands

```sh
make -C paddock release-check \
  VERSION=0.1.0-rc1 \
  COMMIT="$(git rev-parse --short HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

make -C paddock release-artifacts \
  VERSION=0.1.0-rc1 \
  COMMIT="$(git rev-parse --short HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```
