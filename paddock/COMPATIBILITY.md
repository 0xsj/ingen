# Paddock compatibility policy

Paddock uses explicit schema identifiers on policies, graph documents, adapter
requests, reports, review artifacts, and release artifacts. A schema identifier
is part of the contract, not display text.

## Current contracts

The machine-readable contracts live in [`spec/`](spec/):

- `paddock.architecture/v1` — policy input;
- `paddock.graph/v1` — language adapter graph output;
- `paddock.graph-request/v1` — external adapter request;
- `paddock.adapter-tests/v1` — external adapter conformance manifest;
- `paddock.adapter-test-result/v1` — adapter conformance evidence;
- `paddock.component-map/v1` — classified component dependency summary;
- `paddock.policy-tests/v1` — policy test manifest input;
- `paddock.policy-test-result/v1` — policy case outcomes and finding rule IDs;
- `paddock.explanation/v1` — agent-facing findings, triage, and remediation;
- `paddock.policy-diff/v1` — normalized policy change evidence;
- `paddock.policy-review/v1` — durable policy change and test decision;
- `paddock.policy-lock/v1` — sealed policy hashes and canonical policy;
- `ingen.ci-result/v1` — shared CI status, provenance, and evidence envelope;
- `paddock.report/v1` — dependency-policy findings and rule summaries;
- `paddock.baseline/v1` — policy-bound accepted finding identities;
- `paddock.release/v1` — published release manifest;
- `paddock.release-verification/v1` — release integrity result.

The CI envelope `ingen.ci-result/v1`, policy test/review/lock artifacts, and
the base Paddock report are defined by the Go validators and the shared InGen
artifact contract. They follow the same versioning rules below.

## v1 rules

- Producers must emit the declared schema identifier exactly.
- Consumers must reject an unknown schema version rather than silently
  interpreting it as an older contract.
- Additive fields may be introduced only when existing consumers can safely
  ignore them; required-field or semantic changes require a new schema version.
- A changed meaning, field type, identifier, exit-code contract, or capability
  interpretation requires a new version.
- Graph adapters own language analysis, not policy semantics. New edge kinds or
  source units must be advertised through capabilities and validated by the
  request/response contract.
- Canonical policy changes invalidate policy locks even when YAML formatting or
  comments are unchanged; the exact policy file hash also remains part of the
  lock provenance.
- Machine-readable output must remain deterministic apart from explicitly
  time-based provenance fields such as CI artifact timestamps.

## Change procedure

Before changing a v1 contract:

1. update the schema or validator and its fixtures;
2. add a compatibility or rejection test;
3. update the relevant documentation and example artifact;
4. decide whether the change is additive or requires a new schema identifier;
5. run the full Paddock test and vet checks.

This keeps an LLM or external adapter from depending on undocumented behavior,
and keeps policy review evidence portable across Paddock versions.
