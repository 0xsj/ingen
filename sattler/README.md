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
explicit warning instead of hiding the missing detail.

```sh
go run ./sattler/cmd/sattler compare before.json after.json
go run ./sattler/cmd/sattler compare --format json before.json after.json
```

The comparison report is currently `ingen.sattler-comparison/v0`; it is a
working seam for exploration, not a compatibility promise. Sattler still does
not infer causation or reinterpret a producer's verdict.
