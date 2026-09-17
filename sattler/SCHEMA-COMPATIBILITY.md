# Sattler schema compatibility

Status as of 2026-09-17: twelve local machine-readable shapes are compatibility-
treated and the remaining Sattler projections are provisional.

## Compatibility-treated

`ingen.sattler-error/v0` is used by every JSON-mode CLI operation. Its
top-level fields are `schema`, `operation`, and `errors`; each error has a
required `code` and `message`, plus an optional `path`. Unknown top-level or
error fields are rejected by the checked-in schema
[`spec/sattler-error-v0.schema.json`](spec/sattler-error-v0.schema.json), and
the writer validates the envelope before emitting it.

This is a local compatibility treatment of the existing `v0` identifier. A
future breaking change needs a new schema identifier and migration notes.

`ingen.sattler-comparison/v0` is the producer-neutral comparison envelope
used by the root `compare` command. Its top-level fields and interpreted
boundary summaries are defined by
[`spec/sattler-comparison-v0.schema.json`](spec/sattler-comparison-v0.schema.json),
and the JSON writer validates both artifact summaries and change accounting
before emission. `before` and `after` retain shared envelope metadata; nested
producer reports are not copied into this shape.

`ingen.sattler-sorna-run-comparison/v0` is the standalone Sorna subject-run
projection. Its contract bounds contract identity, verdict, rule summaries,
mutation linkage, assurance, lifecycle, and stable change accounting in
[`spec/sattler-sorna-run-comparison-v0.schema.json`](spec/sattler-sorna-run-comparison-v0.schema.json).
Request, observation, and assertion payloads remain outside the contract. The
JSON writer validates run summaries and the changed-rule index without
reinterpreting Sorna's verdict.

`ingen.sattler-nublar-run-comparison/v0` is the standalone Nublar coordination
projection. Its contract bounds workflow identity, workflow file identity,
coordinator status, checks, optional external correlation, completion context,
and stable change accounting in
[`spec/sattler-nublar-run-comparison-v0.schema.json`](spec/sattler-nublar-run-comparison-v0.schema.json).
Nested producer results remain opaque; the JSON writer validates the
coordination summary without assigning improvement or regression meaning.

`ingen.sattler-lockwood-custody-comparison/v0` is the standalone Lockwood
custody projection. Its contract bounds custody identity, receipt timing,
producer/source metadata, integrity status, artifact digest identity, and
stable change accounting in
[`spec/sattler-lockwood-custody-comparison-v0.schema.json`](spec/sattler-lockwood-custody-comparison-v0.schema.json).
Lockwood remains authoritative for artifact storage and integrity verification;
Sattler records identity observations without treating a digest as proof of
correctness.

`ingen.sattler-bundle-summary/v0` is the compact navigation projection for
bundle consumers. Its subsystem summary map is limited to the supported CI,
Sorna, Nublar, Lockwood, and Amber keys; neutral transitions, change counts,
and identity-only correlations have explicit bounded values in
[`spec/sattler-bundle-summary-v0.schema.json`](spec/sattler-bundle-summary-v0.schema.json).
The JSON writer validates these counts and correlation observations before
emission.

`ingen.sattler-bundle-comparison/v0` is the full bundle projection. Its strict
top-level contract preserves the promoted bundle summary, supported adapter
keys, correlations, filters, and shared adapter comparison envelopes while
allowing adapter-specific producer-owned fields to remain opaque in
[`spec/sattler-bundle-comparison-v0.schema.json`](spec/sattler-bundle-comparison-v0.schema.json).
The JSON writer validates adapter schemas, change accounting, summary
reconciliation, and correlation reconciliation without changing producer
verdict semantics.

`ingen.sattler-comparison-series-summary/v0` is the point-free aggregate
history projection. Its compatibility contract covers entry compatibility
counts, boundary change frequencies, producer-specific mutation and rule
frequencies, identity-observation counts, transition counts, and recurring
warning counts. The JSON writer validates the aggregate relationships without
inferring a trend or quality score.

`ingen.sattler-comparison-series-latest/v0` is the final-point retrieval
projection. Its contract requires the retained point identity, bundle manifest,
`latest: true`, and the already-promoted bundle summary; it does not create a
new verdict or trend classification. The JSON writer validates the point and
its identity-only correlations before emission.

`ingen.sattler-investigation/v0` is the focused neutral investigation
projection. Its compatibility contract bounds rule/mutation findings,
co-observed adapter evidence, source references, confidence states, unknown
detail gaps, and the explicit non-regression/non-causation notice in
[`spec/sattler-investigation-v0.schema.json`](spec/sattler-investigation-v0.schema.json).
Before and after values remain opaque JSON observations; the JSON writer
validates identifiers and evidence references without assigning a quality
ranking or causal meaning.

`ingen.sattler-series-query/v0` is the exact-selector history projection. Its
contract bounds one supported selector, matching point identity and manifest
references, source-preserving match observations, and reconciled point/match
counts in [`spec/sattler-series-query-v0.schema.json`](spec/sattler-series-query-v0.schema.json).
The JSON writer validates exact-match records and retained point summaries
without changing ordering or inferring a trend.

`ingen.sattler-comparison-series/v0` is the ordered history projection. Its
contract bounds unique point identities, bundle manifest references, one final
`latest` marker, promoted point summaries, point-level identity correlations,
and reconciled aggregate counts in
[`spec/sattler-series-v0.schema.json`](spec/sattler-series-v0.schema.json).
The JSON writer validates the aggregate relationships without inferring a
trend, quality score, or causal relationship.

## Provisional

The Amber standalone adapter comparison projection remains provisional. Its
discriminator and core
envelope are checked by
[`spec/sattler-projections-v0.schema.json`](spec/sattler-projections-v0.schema.json),
but its field-level compatibility has not been promoted. Producer-owned
detail, uncertainty, and correlation semantics must be demonstrated by real
consumers before those shapes are closed.

Sattler does not infer compatibility from a field being present in a fixture.
Promotion requires a concrete consumer surface, representative examples, and
an explicit decision about which fields are stable versus producer-owned.
