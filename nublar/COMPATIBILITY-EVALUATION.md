# Nublar compatibility evaluation

Status: evaluated and deferred, 2026-09-17

This evaluation checks whether the legacy one-shot `aggregate` command should
be deprecated and whether Nublar needs a public SDK now that the persisted run
path and GitHub Checks delivery are proven.

## Current usage evidence

| Surface | Evidence | Decision |
| --- | --- | --- |
| `aggregate` CLI | The command and `internal/aggregate` package remain covered by the Nublar test suite. | Retain unchanged for compatibility. |
| Make targets | `nublar-aggregate`, `nublar-aggregate-fresh`, webhook aggregation, Sentinel aggregation, and provider fixture targets still invoke it. | Do not deprecate until those consumers have migrated. |
| New consumer path | The GitHub Actions proof uses `run collect`, `run decision`, and `run deliver`; it does not depend on the aggregate output. | Direct new integrations to the run path. |
| Output contracts | `aggregate` emits the older `ingen.nublar-result/v1` shape; the run path emits `ingen.nublar-run/v1` and `ingen.nublar-decision/v1`. | Do not alias or silently transform the shapes. |
| SDK demand | No Nublar consumer currently requests a Go, TypeScript, or other language binding; consumers invoke the CLI and read JSON. | Defer SDK work. |

## Compatibility decision

Keep `nublar aggregate` as a supported compatibility command. It remains
appropriate for one-shot consumers that need a single aggregate result and do
not need immutable run history, correlation, delivery decisions, or receipts.

Use `nublar run collect` for all new integrations. It is the migration target
because it adds immutable run identity, exact provenance, storage, history,
provider-neutral decisions, and delivery without changing producer-owned
meaning.

No deprecation warning, output rewrite, or schema alias is added in this
slice. The shared status mapping remains `passed → 0`, `failed → 1`, and
`error → 2`, but the two output shapes retain their separate contracts.

## Migration plan

When an existing aggregate consumer is ready to migrate:

1. Replace `aggregate --workflow/--root` with `run collect --workflow/--root`
   and select a local run store.
2. Read `ingen.nublar-run/v1` or export
   `ingen.nublar-decision/v1` instead of parsing
   `ingen.nublar-result/v1`.
3. Preserve the process exit-code handling, then add correlation, history,
   receipts, or GitHub delivery only when the consumer needs them.
4. Keep an independent fixture-backed regression until the old consumer no
   longer reads the aggregate shape.

Deprecation requires evidence that the active Make targets and documented
consumers have migrated, a replacement acceptance proof for each, and an
explicit compatibility window. Until then, `aggregate` receives correctness
maintenance and remains part of `make nublar-freeze-check`.

## SDK decision

Do not add a Nublar SDK or public library facade yet. The stable consumer
boundary is the CLI plus versioned JSON; the internal Go packages are not a
public API promise. Reopen SDK work only when a named consumer demonstrates
that process invocation is insufficient and specifies its language, versioning,
error, and contract-test requirements.

## Explicit non-goals

- Converting aggregate output into run output implicitly.
- Removing aggregate targets or historical documentation in this slice.
- Publishing Nublar's internal Go packages as a supported SDK.
- Adding bindings because another InGen component happens to have an SDK.
