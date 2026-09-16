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

Sattler now has a deliberately provisional local comparison surface for the
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
go run ./sattler/cmd/sattler custody compare before-custody.json after-custody.json
go run ./sattler/cmd/sattler provenance compare before-provenance.json after-provenance.json
go run ./sattler/cmd/sattler bundle compare comparison.json
```

The comparison report is currently `ingen.sattler-comparison/v0`; it is a
working seam for exploration, not a compatibility promise. Sattler still does
not infer causation or reinterpret a producer's verdict.

Nublar collection runs can be compared at the coordinator boundary with
`sattler run compare`. This reports workflow identity, coordinator verdicts,
and check-state changes while leaving nested producer results opaque.

Lockwood custody records can be compared with `sattler custody compare`. This
reports custody and integrity status, producer/source metadata, and whether
the stored artifact digest stayed the same or was replaced. A digest remains
an identity reference, not proof of correctness.

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
    "nublar_run": "before-run.json"
  },
  "after": {
    "ci_result": "after-ci.json",
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

Manifests are validated before any artifact is opened. Wrong schemas,
incomplete pairs, and empty manifests produce stable issue codes such as
`invalid-schema`, `incomplete-pair`, and `no-artifact-pairs`. When `--format
json` is selected, operation failures are emitted on stderr as the
`ingen.sattler-error/v0` envelope with an operation name and `errors` array.
