# Nublar

Nublar is InGen's CI and delivery surface. Its first implementation slice is a
small coordinator that aggregates shared CI result envelopes. It may eventually
provide pull-request gates, scheduled mutation campaigns, hosted reports,
retention, and delivery-system integrations.

Nublar should consume Sorna's CLI, run specifications, evidence bundles, and
the shared [`ingen.ci-result/v1`](../core/CI-RESULT-SPEC.md) envelope produced
by tools such as Paddock. It should not reimplement architecture evaluation,
contract evaluation, or mutation semantics.

For Paddock, Nublar should store and surface the result status, exit code,
input hashes, deterministic report, and explanation. The nested Paddock report
remains the source of architectural findings; Nublar is a CI consumer, not a
second architecture engine.

## Workflow declaration

The workflow file declares which producer results a CI run expects:

```yaml
schema: ingen.nublar-workflow/v1
id: document-pipeline-ci
checks:
  - id: mutation-provider-review
    tool: sorna
    result: .artifacts/document-pipeline-provider-review-ci-result.json
    required: true
  - id: behavioral-verification
    tool: sorna
    result: .artifacts/document-pipeline-ci-result.json
    required: true
```

Checks are required by default. An explicitly false `required` value makes a
check optional. Paths are relative to the workspace root supplied to Nublar.

```sh
go run ./nublar/cmd/nublar workflow validate nublar/workflows/document-pipeline.yaml
go run ./nublar/cmd/nublar aggregate \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root . \
  --output .artifacts/nublar-result.json
```

Missing required results produce an `error` aggregate with exit code `2`.
Missing optional results are recorded as warnings. A result whose envelope
claims a different producer than the workflow declares is also an error. The
aggregate records the workflow path and SHA-256 so the collection policy is
part of the result provenance. Each consumed CI-result file is also recorded
with its own SHA-256.

The document-pipeline workflow collects Sorna's provider-preflight envelope as
well as its behavioral-verification envelope. Nublar preserves both complete
producer results and only composes their statuses; the provider review remains
the authority for capability and plan-binding findings.

## Aggregate shared results

```sh
go run ./nublar/cmd/nublar aggregate \
  --output .artifacts/nublar-result.json \
  .artifacts/document-pipeline-ci-result.json \
  .artifacts/another-tool-ci-result.json
```

The command validates every `ingen.ci-result/v1` input, preserves the complete
producer artifact under `results`, and returns the highest envelope severity:
`error` (exit `2`) > `failed` (exit `1`) > `passed` (exit `0`). It does not read
Sorna's mutation fields or Paddock's architecture findings.
