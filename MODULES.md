# InGen module map

This is a rough architectural map, not a commitment to five independently
deployable products. Keep the implementation combined where the code and
lifecycle are naturally shared. Split a surface only when it has a distinct
trust boundary, deployment model, ownership model, or user workflow.

## Shared foundation

### `core/`

Language-neutral artifacts and protocols shared by every InGen surface:

- canonical contract representation and versioning;
- run, lifecycle, and access-event records;
- mutation and rule-result schemas;
- evidence manifests, hashes, and compatibility rules.

This should remain small and stable. It is the interchange layer, not a second
verification engine.

## Verification

### `sorna/`

The standalone verification laboratory. Sorna should be usable from a local
terminal or CI without Herdr. It will eventually own:

- contract validation, sealing, and canonicalization;
- isolated oracle generation and freezing;
- public-interface adapters;
- language-specific mutation providers that prepare isolated subject variants;
- baseline verification;
- behavioral, contract, and implementation mutation campaigns;
- evidence bundles, replay, and assurance reporting.

The `internal/` directories are planned Go package boundaries. The oracle
runtime's access restrictions are a real runtime boundary even if Sorna and
other InGen code initially live in one repository.

## Interactive workflow

### `herdr-sentinel/`

The Herdr-native control room for the Sorna workflow. Sentinel should own:

- the visible contract workspace;
- agent roles, sessions, and worktrees;
- capability-policy setup;
- lifecycle, blocked decisions, and human approvals;
- invoking Sorna and surfacing its artifacts and results.

Sentinel should not redefine contract semantics, mutation operators, or
authoritative evidence.

## Future surfaces

### `hammond/`

Placeholder for contract intent and governance across projects: shared
contract registries, review history, approval policy, amendments, and lineage.
The local contract workspace remains a Sentinel feature even if Hammond later
provides a broader registry or service.

### `nublar/`

The first CI and delivery slice is an envelope coordinator plus a workflow
declaration. It validates expected producer results, preserves
`ingen.ci-result/v1` artifacts, and composes their shared status and exit-code
semantics. Future work includes pull-request gates, scheduled mutation
campaigns, artifact retention, hosted reports, and integrations with delivery
systems. Nublar should remain a consumer of Sorna's CLI and evidence protocol,
not a competing verifier.

## Integrations

### `integrations/`

Optional adapters for external tools and environments. A Stryker adapter may
eventually import or translate source-level mutation results, but Stryker is a
reference/integration option—not an InGen dependency or architectural model.

## Learning corpus

### `notes/`

The notes protocol is part of the project, not a later documentation task. It
captures language concepts, substrate behavior, techniques, patterns, and
verification principles that cannot be recovered from the code alone. See
[NOTES.md](NOTES.md) for the protocol and [notes/README.md](notes/README.md) for
the corpus entry point.

## Current implementation rule

Start as one Go-oriented monorepo with shared packages and a Sorna CLI. Add
named verticals only when a real workflow or deployment need appears. The
protocol and artifacts should be stable before building hosted services or
multiple SDKs.
