# Sattler roadmap

Status as of 2026-09-17: the local Sattler comparison foundation is complete
and there is no Sattler-specific blocker. Before this roadmap was added, the
Sattler code was clean on `dev`, which is synchronized with `origin/dev`. The
repository may contain unrelated edits outside Sattler; they are intentionally
not part of this roadmap.

## What is complete

### Comparison foundation

- Compares shared `ingen.ci-result/v1` artifacts with deterministic text and
  JSON output.
- Separates verdict, context, contract, plan, provider, environment, and
  producer-report changes.
- Uses stable change IDs such as `verdict.status` and `inputs.contract`.
- Applies conservative compatibility rules: producer and result kind must
  remain compatible.
- Preserves timestamps, opaque producer reports, uncertainty, and warnings.
- Compares artifact references using conservative identity relations such as
  `same-bytes`, `replaced`, `added`, `removed`, and `unknown`.

### Producer and evidence adapters

- Sorna mutation-campaign summaries, including changed mutation outcomes.
- Nublar run summaries, workflow/check changes, and optional correlation IDs.
- Lockwood custody summaries, source linkage, receipt timing, and digest
  identity.
- Amber provenance summaries, retry/replay transitions, and correlation IDs.

### Bundle and history surfaces

- Comparison manifests align optional CI, Sorna, Nublar, custody, and
  provenance pairs.
- Bundle summaries retain subsystem compatibility, transitions, warnings, and
  change counts.
- Bundle correlations relate Nublar to Lockwood and Amber as identity
  observations only; they do not claim causation.
- Ordered series manifests preserve history order and expose stable boundary
  change frequencies.
- Series reports include mutation change frequencies, correlation counts,
  Sorna rule change frequencies, transition counts by subsystem, recurring
  warning counts, and an explicit latest point.
- Series projections support full, summary-only, and latest-only output.
- Targeted series queries support exact rule, mutation, contract, provider, and
  workflow selectors while preserving point and artifact references.

### Operator surface and safeguards

- CLI commands exist for CI, Sorna, Nublar, Lockwood, Amber, bundle, and series
  comparisons, plus the neutral investigation projection.
- Text and JSON output are deterministic.
- Repeatable/comma-separated `--change-id` filtering is available across the
  comparison surfaces.
- Machine-readable operation failures use `ingen.sattler-error/v0`.
- Checked-in JSON Schema contracts cover strict input manifests and the core
  envelope of every emitted projection; deterministic golden output tests
  cover representative text and JSON ordering.
- Manifest decoding rejects unknown fields and trailing JSON documents, while
  adapter boundaries keep wrong producer schemas explicit.
- The `ingen.sattler-error/v0` envelope is the first compatibility-treated
  output shape, with a strict schema and runtime writer validation.
- The `ingen.sattler-comparison/v0` producer-neutral envelope is now also
  compatibility-treated, with strict core fields and change accounting.
- The compact `ingen.sattler-bundle-summary/v0` navigation projection is now
  compatibility-treated, with bounded subsystem and correlation fields.
- The point-free `ingen.sattler-comparison-series-summary/v0` projection is
  now compatibility-treated, with reconciled aggregate count maps.
- The `ingen.sattler-comparison-series-latest/v0` retrieval projection is now
  compatibility-treated, requiring a marked final point and bundle summary.
- The `ingen.sattler-investigation/v0` neutral findings projection is now
  compatibility-treated, with bounded evidence and uncertainty fields.
- The `ingen.sattler-series-query/v0` exact-selector projection is now
  compatibility-treated, with reconciled match counts and source references.
- The `ingen.sattler-comparison-series/v0` ordered history projection is now
  compatibility-treated, with latest-marker and aggregate reconciliation.
- The `ingen.sattler-bundle-comparison/v0` full bundle projection is now
  compatibility-treated, with opaque adapter detail and strict reconciliation.
- The `ingen.sattler-sorna-run-comparison/v0` standalone Sorna projection is
  now compatibility-treated, with strict run summaries and rule accounting.
- The `ingen.sattler-nublar-run-comparison/v0` standalone Nublar projection is
  now compatibility-treated, with strict workflow and check accounting.
- The `ingen.sattler-lockwood-custody-comparison/v0` standalone Lockwood
  projection is now compatibility-treated, with strict custody, receipt,
  producer, integrity, digest-identity, and change accounting.
- Focused tests and vet pass for `./sattler/...`; formatting and diff checks
  pass.

The full repository test run is not currently a clean environmental signal:
an unrelated Hammond HTTP test cannot bind its `httptest` listener inside the
sandbox. The failure is outside Sattler and does not block focused development.

## Completed next slice: Sorna rule-level adapter

The published `ingen.run/v1` record is now available through the standalone
`sattler sorna compare` command and the `ingen.sattler-sorna-run-comparison/v0`
report. It compares contract identity, contract verdict, summary counters,
rule statuses, optional mutation linkage, assurance, and selected lifecycle
context. Rule request, observation, and assertion payloads remain opaque.

The adapter preserves `error`, `inconclusive`, and `skipped` states, uses stable
rule IDs such as `rules.<rule_id>.status`, supports the common `--change-id`
filter, and has schema-shaped fixture coverage with deterministic JSON/text
output.

## Completed next slice: cross-artifact fixture

The reviewable `testdata/cross-artifact` fixture now combines CI, raw Sorna,
Nublar, Lockwood, and Amber before/after records with an ordered series
manifest. Integration coverage exercises bundle subsystem summaries, identity
replacement, explicit cross-artifact correlations, a preserved producer-detail
warning, Sorna rule/mutation changes, and series aggregation. The raw Sorna
pair is exercised both through the standalone adapter and through the bundle
boundary.

## Completed next slice: Sorna bundle and series integration

Comparison manifests now accept optional `sorna_run` before/after paths. Bundle
comparisons load those `ingen.run/v1` records through the standalone adapter,
include the independent `sorna_run` subsystem summary/detail, and keep rule
change IDs visible in full and text output. Bundle filtering applies the same
stable IDs to Sorna changes and changed-rule detail. Series points and summaries
retain a separate Sorna rule-change index while generic `changes_by_id` still
aggregates all adapter boundary changes.

## Completed next slice: deterministic investigation projection

The `sattler investigate` command and `ingen.sattler-investigation/v0` report
now provide a focused projection over a bundle. It keeps rule and mutation
findings separate from co-observed adapter evidence changes, retains before and
after source paths, and records direct, partial, or unknown confidence states.
Missing Sorna or producer-detail inputs remain explicit unknowns. The report is
deterministic and carries the non-causal, non-regression interpretation notice.

## Completed next slice: targeted history queries

The `series query` command and `ingen.sattler-series-query/v0` projection now
support one exact selector at a time for a rule, mutation, contract, provider,
or workflow. Matching points retain ordered series identity, latest markers,
bundle manifest paths, and source artifact paths. Query matches remain separate
from stable boundary change IDs and producer-owned detail.

## Completed next slice: schema hardening

Checked-in contracts now cover the comparison and series input manifests plus
the core envelope of every emitted Sattler JSON projection. Input manifests
are strict at the Sattler boundary; malformed JSON, unknown fields, trailing
documents, partial pairs, empty series manifests, duplicate IDs, and missing
entry paths remain explicit validation failures. Wrong producer records remain
adapter errors, while mixed shared CI envelopes remain explicit incompatible
comparisons. Representative text and JSON goldens lock deterministic ordering.

The provisional `v0` output shapes are not yet field-level compatibility
guarantees. Their discriminator and required envelope fields are checked;
additional projection fields remain permitted until real consumers justify
promotion of an individual shape.

## Completed next slice: error-envelope promotion

The shared `ingen.sattler-error/v0` envelope is now compatibility-treated for
local CLI consumers. Its required fields and error issue shape are explicit in
[`SCHEMA-COMPATIBILITY.md`](SCHEMA-COMPATIBILITY.md) and
`spec/sattler-error-v0.schema.json`; runtime writing rejects incomplete
envelopes before emission. A breaking change requires a new schema identifier
and migration notes.

## Completed next slice: base comparison promotion

The root `compare` surface now treats `ingen.sattler-comparison/v0` as a local
field-level compatibility contract. The checked-in schema covers shared
artifact summaries, neutral transitions, stable changes, and change counts;
runtime writing rejects incomplete summaries, invalid transitions, and change
accounting that does not match the emitted changes. Producer-owned detail
remains outside this promoted shape.

## Completed next slice: bundle summary promotion

The compact `ingen.sattler-bundle-summary/v0` projection is now
compatibility-treated for navigation consumers. Its schema bounds supported
subsystems, neutral transition classifications, change-count invariants, and
identity-only correlation observations. Runtime JSON writing rejects empty or
unsupported subsystem summaries, invalid count accounting, and invalid
correlations. Adapter-specific detail remains producer-owned; the full bundle
projection's top-level promotion is complete below.

## Completed next slice: series summary promotion

The point-free `ingen.sattler-comparison-series-summary/v0` projection is now
compatibility-treated for aggregate history consumers. Its schema and runtime
validation reconcile compatible/incompatible entries, boundary change totals,
correlation observations, transition counts, and warning/frequency maps
without inferring a trend or quality score.

## Completed next slice: latest-point promotion

The `ingen.sattler-comparison-series-latest/v0` retrieval projection is now
compatibility-treated. Its schema and runtime validation require a non-empty
point identity, bundle manifest, `latest: true`, a valid promoted bundle
summary, and bounded identity-only correlations. It remains a retrieval
surface and does not infer a new verdict or trend.

## Completed next slice: investigation promotion

The `ingen.sattler-investigation/v0` projection is now compatibility-treated.
Its schema and runtime validation bound rule and mutation findings, co-observed
adapter evidence, source references, confidence states, unknown detail gaps,
and the explicit non-regression/non-causation notice. Before and after values
remain opaque observations; evidence references must resolve within the report.

## Completed next slice: targeted-query promotion

The `ingen.sattler-series-query/v0` exact-selector projection is now
compatibility-treated. Its schema and runtime validation bound supported query
kinds, source-preserving exact matches, retained point identity and manifests,
and reconciled point/match counts. It preserves series order and does not
reinterpret producer-owned fields or infer a trend.

## Completed next slice: full ordered-series promotion

The `ingen.sattler-comparison-series/v0` ordered history projection is now
compatibility-treated. Its schema and runtime validation bound unique point
identities, bundle manifest references, one final `latest` marker, promoted
point summaries, point-level correlations, and reconciled aggregate counts.
It preserves order and does not infer a trend, quality score, or causation.

## Completed next slice: full bundle-comparison promotion

The `ingen.sattler-bundle-comparison/v0` full bundle projection is now
compatibility-treated. Its schema and runtime validation bound supported
subsystem keys, shared adapter comparison envelopes, promoted summary and
correlation projections, filters, and reconciled change accounting while
leaving adapter-specific producer-owned fields opaque.

## Completed next slice: Sorna run comparison promotion

The `ingen.sattler-sorna-run-comparison/v0` standalone projection is now
compatibility-treated. Its schema and runtime validation bound contract
identity, verdict, rule summaries, mutation linkage, assurance, lifecycle,
stable changes, and the changed-rule index. Request, observation, and
assertion payloads remain outside the contract, and Sattler does not reinterpret
the Sorna verdict.

## Completed next slice: Nublar run comparison promotion

The `ingen.sattler-nublar-run-comparison/v0` standalone projection is now
compatibility-treated. Its schema and runtime validation bound workflow and
workflow-file identity, coordinator status, check summaries, optional
correlation, completion timestamps, and stable change accounting. Nested
producer results remain opaque, and Sattler does not label transitions as
improvements or regressions.

## Completed next slice: Lockwood custody comparison promotion

The `ingen.sattler-lockwood-custody-comparison/v0` standalone projection is
now compatibility-treated. Its schema and runtime validation bound custody
identity, receipt timestamps, producer/source metadata, integrity status,
`sha256:` artifact digest identity, and stable change accounting. Lockwood
remains authoritative for storage and verification semantics.

## Next recommended slice

### 1. Promote the Amber provenance comparison shape

- Gather consumer feedback and examples for
  `ingen.sattler-amber-provenance-comparison/v0`.
- Decide which provenance, retry/replay, work/execution, and correlation fields
  are stable while keeping provenance authority and causation semantics outside
  Sattler.
- Add a strict schema and migration notes only after that decision.

## Guardrails

- Sattler observes and compares; Sorna remains authoritative for execution and
  verdict semantics.
- Correlation is identity evidence, not proof of causation.
- Compatibility, transition, and change counts are navigation metadata, not a
  single quality score.
- Unknown, unavailable, inconclusive, and incomplete evidence must remain
  visible.
- Keep the local artifact model stable before adding hosted analytics or a
  dashboard.

## Verification gate for each slice

Every implementation slice should leave these checks green:

```sh
GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" go test ./sattler/...
GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" go vet ./sattler/...
gofmt -d sattler/*.go sattler/cmd/sattler/*.go
git diff --check -- sattler
```
