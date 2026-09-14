# Paddock status

Last updated: 2026-09-14

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

## Implemented

### Analysis and enforcement

- Go package graph adapter.
- TypeScript/JavaScript file graph adapter.
- Python file graph adapter.
- External adapter protocol with capability negotiation.
- Component classification and language-neutral policy format.
- Layer direction, allow/deny dependency, cross-context, cycle, coverage,
  required-dependency, and unresolved-import rules.
- Warnings, waivers, baselines, and deterministic exit codes.

### Review and CI workflow

- `check`, `graph`, `init`, `baseline`, `ci`, and `explain` commands.
- Policy tests with expected pass/fail/error cases and required rule IDs.
- Policy diff and durable policy review artifacts.
- Policy sealing and lock verification.
- CI result artifacts using `ingen.ci-result/v1`.
- Agent-facing text and JSON explanations.

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
- Overwatch backend review policy, lock, and policy tests.
- Overwatch UI draft policy, layered proposal, lock, and policy tests.
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

1. Review the current working-tree changes and decide whether to publish
   `paddock-v0.1.0-rc1`.
2. Push the tag only after the release assets and GitHub workflow are approved.
3. Observe the hosted workflow end to end: build, verify, upload, and release.
4. Use the published binary in at least one external repository or project
   CI job.
5. Record adapter, path-resolution, performance, and policy-authoring issues
   from that real run before changing the core schemas.

### Likely near-term work

- Decide whether the Overwatch backend findings should be fixed, waived, or
  temporarily baselined.
- Decide whether the Overwatch UI component vocabulary needs finer-grained
  presentation roles.
- Freeze and document compatibility expectations for `paddock.graph/v1`, the
  policy format, and the CI/release artifact schemas.
- Add release workflow smoke coverage if hosted CI exposes issues not visible
  locally.
- Add a component dependency matrix or graph summary optimized for human and
  agent review.

### Defer until real usage asks for them

- Additional language adapters.
- More architecture rule kinds.
- Deeper LLM-specific features.
- Hosted dashboards or centralized policy services.
- Large-scale performance work beyond the first real monorepo measurements.

## Recommended stopping point

Do not add more core Paddock features until the release candidate has been
consumed by at least one real project. The next decision should be based on
observed adoption friction, false positives, adapter gaps, and CI ergonomics.

At that point, either promote Paddock to a small stable `0.1` release or
revise the policy/protocol surface before expanding the feature set.

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
