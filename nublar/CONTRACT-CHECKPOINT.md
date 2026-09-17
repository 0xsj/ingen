# Nublar local contract checkpoint

As of 2026-09-17, Nublar's first product-shaped slice is a local,
filesystem-backed coordinator. It collects already-produced CI envelopes,
records one immutable run, exposes a read-only history, and projects a
provider-neutral delivery decision. A separately tested GitHub Checks adapter
publishes that decision without changing the v1 run, decision, or receipt
schemas.

The resulting freeze boundary and change rule are recorded in
[`FREEZE-RECORD.md`](FREEZE-RECORD.md).
Use [`CONSUMER-REQUEST-TEMPLATE.md`](CONSUMER-REQUEST-TEMPLATE.md) to capture
the next concrete consumer requirement before changing this checkpoint.

## Stable boundaries

| Boundary | Schema | Current contract |
| --- | --- | --- |
| Workflow declaration | `ingen.nublar-workflow/v1` | Required/optional result paths and producer identities. |
| Run record | `ingen.nublar-run/v1` | One immutable collection attempt with exact workflow/result hashes, opaque producer artifacts, decision state, and optional external correlation. |
| Delivery decision | `ingen.nublar-decision/v1` | Compact projection with references, status, issues, and optional correlation; producer reports are excluded. |
| Delivery receipt | `ingen.nublar-delivery-receipt/v1` | Independent accepted/failed outcome for one delivery attempt. |

## Consumer path

The covered local workflow is:

```text
validate workflow
    → collect producer envelopes
    → persist immutable run
    → list/show/filter history
    → project decision
    → optionally deliver webhook or GitHub check and export/store receipt
```

`run_id` remains the local per-attempt identity. When an external system
provides one, `correlation.system`, `correlation.id`, and
`correlation.attempt` are carried separately. Correlation filters are exact,
composable, and read-only.

## Guarantees covered by the checkpoint

- v1 workflow, envelope, run, decision, and receipt shapes are closed where
  their contracts require it.
- Run and artifact loading reject unknown fields and multiple JSON values.
- Workflow and consumed artifact bytes are bound to SHA-256 references.
- Failed producer runs and collection-error runs are persisted before their
  decision exit codes are returned.
- Storage publication is immutable and deterministic; delivery failure does
  not mutate the stored run, and publisher-reached receipts can be persisted
  independently in a content-addressed store.
- Receipt history is strict-loaded and supports exact, composable filters by
  run ID, status, and transport.
- The end-to-end consumer test covers collection, storage, projection, and
  generic webhook delivery with an in-memory transport, while the CLI delivery
  regressions cover accepted and failed receipts and unchanged run state after
  delivery failure. Repeating the same delivery keeps the same idempotency key
  and records independent receipt outcomes.
- The GitHub Checks adapter contract covers completed-check status mapping,
  token injection, same-run remote lookup/update, bounded transient retries,
  failed receipts, and omission of producer-owned reports from the published
  check output.

## Verification

```sh
make nublar-check
```

This runs Nublar's race tests, static analysis, and schema syntax checks
without launching any producer workflow.

The provider-neutral local consumer example is
[`examples/consumer/README.md`](examples/consumer/README.md). It exercises the
same collect/store/decision path from a CI-style shell boundary.
Run `make nublar-consumer-check` to verify that example against the checked-in
fixtures, including persisted run and decision outputs for failed, passed,
missing-artifact, and malformed-envelope paths, plus filtered `run list` and
exact `run show` history reads. The consumer check also confirms that listing
a failed run succeeds while showing it returns the stored `failed/1` code.
Run `make nublar-freeze-check` to execute this consumer check together with
the race, vet, schema, and diff-whitespace gates.

## Deferred until a concrete consumer requires them

Producer execution, hosted Nublar APIs, remote storage, retention, pagination,
summary-only responses, and durable retry stores remain outside this checkpoint.
GitHub-specific comments, annotations, approvals, branch-rule management, and
other provider delivery formats remain deferred. New fields or behavior should
be added through a documented contract decision and an executable consumer
test, rather than speculative infrastructure.
