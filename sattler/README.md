# Sattler

Sattler is InGen's verification intelligence and regression-analysis surface.

It compares verified runs over time so that InGen can explain not only whether
a system passed, but how its reliability, contract sensitivity, evidence, and
provenance changed between runs.

The project is named for Dr. Ellie Sattler: careful observation, context, and
interpretation matter as much as the result of an experiment.

## Why it exists

Sorna can execute a verification campaign and produce a verdict. Lockwood can
preserve the artifacts. Amber can describe how the work and artifacts relate.
Sattler would make those results comparable across commits, agents, contracts,
providers, and environments.

It should help answer questions such as:

- Did this implementation improve or regress against the same contract?
- Which contract rules or mutations repeatedly fail or survive?
- Did a change reduce the quality or completeness of the evidence?
- Which agent, prompt, provider, or workflow is associated with recurring
  defects?
- What changed between two otherwise similar verified runs?

## Role in InGen

Sattler is an analysis boundary, not a second verification engine:

- **Malcolm** describes executable specifications and independent contracts.
- **Hammond** governs contract ownership, versions, and approval.
- **Sorna** executes behavioral verification and mutation campaigns.
- **Amber** records provenance across work, executions, agents, and artifacts.
- **Lockwood** preserves evidence bundles and content-addressed artifacts.
- **Nublar** coordinates workflows and consumes CI-facing results.
- **Sattler** compares history, explains regressions, and surfaces trends.

The intended flow is:

```text
verified runs + provenance + custody artifacts
        -> Sattler comparison and analysis
        -> regression report, trend, or investigation
```

Sattler should consume stable artifacts and preserve their identities. It may
interpret differences, but it must not silently rewrite a Sorna verdict,
change a contract, or claim that correlation proves causation.

## Example direction

```sh
sattler compare run-a run-b
sattler benchmark --suite document-pipeline
sattler explain-regression run-2026-09-16
sattler trace-failure --rule document.process.valid-completes
```

A comparison might report that a contract moved from six of six meaningful
mutations killed to four of six, identify the surviving mutations, and link the
change to the relevant contract, provider, source, evidence, and provenance
artifacts.

## Initial scope

The first version should stay deliberately small:

- compare two compatible Sorna or Nublar result envelopes;
- distinguish verdict changes from contract, plan, provider, and environment
  changes;
- compare mutation outcomes and rule-level failures;
- link differences to Amber provenance and Lockwood artifact identities;
- emit deterministic human-readable and machine-readable reports;
- preserve uncertainty when a difference cannot be attributed confidently.

## Non-goals

Sattler is not intended to:

- replace Sorna's execution or verdict semantics;
- store artifacts that belong in Lockwood;
- govern contract changes that belong in Hammond;
- infer causation from a single correlation;
- reduce verification quality to one score or mutation percentage;
- become a hosted analytics product before the local artifact model is stable.

## Working first slice

Sattler now has a compatibility-treated local comparison surface for the
shared `ingen.ci-result/v1` envelope. It compares two valid result files,
reports verdict changes separately from source and input changes, and
fingerprints producer-owned reports without interpreting their semantics. When
both inputs are Sorna mutation campaigns, it additionally summarizes the
producer-owned plan and mutation outcome changes. If a recognized producer
report cannot be decoded, Sattler keeps the generic comparison and emits an
explicit warning instead of hiding the missing detail. Common input names are
classified as `contract`, `plan`, `provider`, or `environment`; unknown names
remain generic `input` changes.

Changed file references also carry an identity relation. Matching SHA-256
digests are `same-bytes` even when paths move; differing known digests are
`replaced`; missing and newly introduced references are `removed` or `added`.
Sattler reports `unknown` when the available references cannot establish a
byte identity.

Compatibility is intentionally narrow: a CI comparison requires the same
producer and result kind, while source, workflow-file, and input changes remain
comparable context and are reported explicitly.

Comparison summaries retain producer timestamps, Nublar completion boundaries,
and Lockwood receipt timestamps for historical analysis. These times are
context, not regression findings.

Each comparison also includes a deterministic `change_summary` with total and
per-category counts. It is a navigation aid, not a quality score.

```sh
go run ./sattler/cmd/sattler compare before.json after.json
go run ./sattler/cmd/sattler compare --format json before.json after.json
go run ./sattler/cmd/sattler run compare before-run.json after-run.json
go run ./sattler/cmd/sattler sorna compare before-sorna-run.json after-sorna-run.json
go run ./sattler/cmd/sattler custody compare before-custody.json after-custody.json
go run ./sattler/cmd/sattler provenance compare before-provenance.json after-provenance.json
go run ./sattler/cmd/sattler bundle compare comparison.json
go run ./sattler/cmd/sattler bundle compare --summary-only --format json comparison.json
go run ./sattler/cmd/sattler bundle compare --change-id verdict.status comparison.json
go run ./sattler/cmd/sattler investigate comparison.json
go run ./sattler/cmd/sattler series compare series.json
go run ./sattler/cmd/sattler series compare --summary-only --format json series.json
go run ./sattler/cmd/sattler series compare --latest-only series.json
go run ./sattler/cmd/sattler series compare --change-id verdict.status series.json
go run ./sattler/cmd/sattler series query --rule document.process.valid-completes series.json
```

The comparison report is `ingen.sattler-comparison/v0`, the first
producer-neutral projection with a local field-level compatibility treatment.
Its nested producer-owned detail remains outside the contract. Sattler still
does not infer causation or reinterpret a producer's verdict.

Sorna subject-run records can be compared at the rule boundary with
`sattler sorna compare`. The adapter understands the published
`ingen.run/v1` shape, compares contract identity, verdict and summary context,
and reports rule status changes while keeping request, observation, and
assertion details opaque. The standalone `ingen.sattler-sorna-run-comparison/v0`
JSON projection is compatibility-treated; its strict contract validates run
summaries, mutation linkage, assurance, lifecycle, and changed-rule accounting.
Rule-level IDs follow the form
`rules.<rule_id>.status`.

Nublar collection runs can be compared at the coordinator boundary with
`sattler run compare`. This reports workflow identity, coordinator verdicts,
and check-state changes while leaving nested producer results opaque. The
standalone `ingen.sattler-nublar-run-comparison/v0` JSON projection is
compatibility-treated; its strict contract validates workflow and file
identity, checks, correlations, completion context, and change accounting.

Lockwood custody records can be compared with `sattler custody compare`. This
reports custody and integrity status, producer/source metadata, and whether
the stored artifact digest stayed the same or was replaced. A digest remains
an identity reference, not proof of correctness. The standalone
`ingen.sattler-lockwood-custody-comparison/v0` JSON projection is
compatibility-treated; its strict contract validates custody identity, digest
identity, receipt/integrity metadata, and stable changes while leaving storage
ownership in Lockwood.

Amber provenance values can be compared with `sattler provenance compare`.
This reports work, execution, and correlation identity plus retry/replay
transition changes. Amber remains authoritative for provenance validation and
causation semantics.

Related artifacts can be aligned with a small JSON comparison manifest:

```json
{
  "schema": "ingen.sattler-comparison-input/v0",
  "before": {
    "ci_result": "before-ci.json",
    "sorna_run": "before-sorna-run.json",
    "nublar_run": "before-run.json"
  },
  "after": {
    "ci_result": "after-ci.json",
    "sorna_run": "after-sorna-run.json",
    "nublar_run": "after-run.json"
  }
}
```

`sattler bundle compare comparison.json` combines whichever complete artifact
pairs are declared. Each subsystem report remains independent and optional.
The bundle also emits a top-level summary with aggregate compatibility,
subsystem-qualified compatibility reasons, total/category change counts, and
per-subsystem change counts. This summary is navigation metadata, not a
quality score or causal conclusion.

The reviewable `testdata/cross-artifact` fixture exercises a before/after bundle
with CI, raw Sorna, Nublar, Lockwood, and Amber artifacts, plus a series
manifest. It intentionally includes an evidence-detail warning, a replaced
contract digest, and an Amber correlation mismatch so uncertainty and identity
observations remain visible. The same raw `ingen.run/v1` pair can still be
compared directly with `sattler sorna compare`; bundle and series reports retain
the Sorna adapter's rule-level changes without copying its opaque request or
observation payloads.

Use `--summary-only` to emit the compact `ingen.sattler-bundle-summary/v0`
projection containing the bundle summary and correlations without the full
adapter reports. Both the summary and full bundle projections are
compatibility-treated; the full bundle contract keeps adapter-specific fields
opaque while validating their shared comparison envelopes.

Use repeatable or comma-separated `--change-id` values to retain only selected
boundary changes, such as `verdict.status` or `inputs.contract`. Filtered
reports recalculate their summaries and record the active IDs in
`change_id_filter`; CI producer-owned mutation details remain separate, while
Sorna rule changes can be selected with IDs such as
`rules.document.process.valid-completes.status`.

When both Nublar and Lockwood inputs are present, the bundle records explicit
before/after observations relating Nublar `run_id` to Lockwood
`source.run_id`. Relations are `exact-match`, `mismatch`, or `unknown` when an
identifier is unavailable; they are identity observations, not causation
claims.

When Nublar records its optional external correlation and Amber provenance is
present, Sattler also relates Nublar `correlation.id` to Amber
`correlation_id` using the same relationship states.

Each adapter comparison also exposes a primary state transition. CI results,
Nublar runs, and custody records use `status`; Sorna subject runs use
`contract_verdict.status`; Amber provenance uses `mode.kind`. The
classification is `unchanged`, `changed`, or `incompatible`, and deliberately
does not label a transition as an improvement or regression.

Observable changes include stable IDs such as `verdict.status` and
`inputs.contract`. These IDs identify the changed field category across runs;
they do not encode the before/after values.

The same `--change-id` selector is available on every standalone comparison
command. Filtered standalone reports recalculate `change_summary` and record
the selected IDs in `change_id_filter`.

`sattler investigate comparison.json` emits the deterministic
`ingen.sattler-investigation/v0` projection over a bundle. It lists explicit
rule and mutation findings, the adapter changes retained as co-observed
evidence, source paths for both sides, and unresolved detail gaps. Findings
carry `direct`, `partial`, or `unknown` confidence based on their available
source paths; these states describe evidence linkage, not verification quality.
The report includes a compatibility-treated notice that co-observed evidence
is not a regression or causation claim. Its strict schema bounds the finding,
evidence, source-reference, and unknown-detail fields while leaving observed
before/after values opaque. The command accepts the same `--change-id` selector
before the manifest path.

An ordered history can aggregate existing bundle manifests:

```json
{
  "schema": "ingen.sattler-comparison-series-input/v0",
  "entries": [
    {"id": "attempt-1", "label": "first attempt", "manifest": "first/comparison.json"},
    {"id": "attempt-2", "label": "second attempt", "manifest": "second/comparison.json"}
  ]
}
```

`sattler series compare series.json` preserves entry order and emits
`ingen.sattler-comparison-series/v0` with compatible/incompatible counts,
aggregate change totals, and `changes_by_id` frequencies. It does not infer a
trend direction or causation. The full ordered-series projection is now
compatibility-treated; its strict contract validates point identity, bundle
references, the final `latest` marker, promoted summaries, and aggregate
reconciliation.
The final ordered point is marked `latest: true` in JSON and `[latest]` in text
so consumers can identify the current endpoint without reinterpreting order.
For Sorna mutation campaigns, each point also exposes the changed mutation IDs
and the series aggregates them as `mutation_changes_by_id`; these are kept
separate from boundary `changes_by_id` because they remain producer-owned
observations.
Point-level identity observations are preserved in both JSON and text output,
including Nublar-to-Lockwood and Nublar-to-Amber correlation relations.
For raw Sorna run pairs, each point also exposes changed rule IDs under
`sorna_rule_change_ids`, with series frequencies under
`sorna_rule_changes_by_id`; the detailed rule transitions remain in the
bundle's `sorna_run` adapter report.
The series summary also counts these observations under `correlations`, split
by correlation kind and relation; `unknown` remains an explicit observation.
It also aggregates adapter transition classifications under
`transitions_by_subsystem`, preserving the neutral `unchanged`, `changed`, and
`incompatible` states.
Recurring non-fatal detail gaps are counted under `warnings_by_message`; these
warnings remain evidence-completeness observations, not verdict changes.
Series filters use the same stable IDs and are recorded in
`change_id_filter`.

Use `series compare --summary-only` to emit the compact
`ingen.sattler-comparison-series-summary/v0` projection containing aggregate
history counts without the ordered point entries. This point-free projection
is compatibility-treated; its count maps and observation summaries are
validated before JSON emission.
Use `series compare --latest-only` to emit the final ordered point as
`ingen.sattler-comparison-series-latest/v0`; this is a retrieval projection,
not a new verdict or trend classification. The two projection flags are
mutually exclusive. This latest-point projection is compatibility-treated and
requires the final point's `latest` marker and bundle reference.

Use `series query` for one exact history selector: `--rule`, `--mutation`,
`--contract`, `--provider`, or `--workflow`. For example,
`series query --rule document.process.valid-completes series.json` returns only
the ordered points where that Sorna rule changed, along with before/after
statuses and source paths. Contract selectors match a contract ID, version
form, digest, or file reference; workflow selectors match an ID, path, or
digest; provider selectors match CI provider file references. Query selectors
are distinct from stable `--change-id` filters and do not rewrite the retained
bundle or producer-owned fields. The `ingen.sattler-series-query/v0` projection
is compatibility-treated; its strict contract validates the selector, exact
matches, source references, retained points, and summary counts.

Manifests are validated before any artifact is opened. Wrong schemas,
incomplete pairs, and empty manifests produce stable issue codes such as
`invalid-schema`, `incomplete-pair`, and `no-artifact-pairs`. When `--format
json` is selected, operation failures are emitted on stderr as the
`ingen.sattler-error/v0` envelope with an operation name and `errors` array.

The checked-in structural contracts live under [`spec/`](spec/). The input
contract [`sattler-input-v0.schema.json`](spec/sattler-input-v0.schema.json)
keeps comparison and series manifests closed and rejects unknown fields;
runtime validation also rejects malformed JSON, incomplete artifact pairs,
empty series manifests, duplicate series IDs, and missing series manifest
paths. The projection contract
[`sattler-projections-v0.schema.json`](spec/sattler-projections-v0.schema.json)
checks every emitted machine-readable surface. Sattler keeps these output
shapes provisional: for the remaining projections, their schema discriminator
and required envelope fields are the current compatibility boundary, while
additional projection fields remain forward-compatible until a shape is
promoted.

The error envelope was the first compatibility-treated output shape because it
is shared by every JSON-mode CLI operation and contains no producer-owned
detail. Its strict contract and promotion decision are recorded in
[`SCHEMA-COMPATIBILITY.md`](SCHEMA-COMPATIBILITY.md); all other projections
remain provisional for now, apart from the base comparison, bundle summary,
series summary, latest-point, investigation, targeted-query, full-series,
full-bundle, Sorna-run, Nublar-run, and Lockwood-custody envelopes described
above.
