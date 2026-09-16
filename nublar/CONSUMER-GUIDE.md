# Nublar consumer handoff

This guide describes how an external CI or delivery system consumes the local
Nublar first slice. Nublar collects producer artifacts; it does not launch the
producer commands that create them.

## Collect a run

The external workflow first publishes complete `ingen.ci-result/v1` files at
the paths declared by `ingen.nublar-workflow/v1`. Nublar then collects and
validates them:

```sh
nublar run collect \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json
```

When the caller has an external attempt identity, add the complete optional
tuple, for example:

```sh
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
```

Nublar keeps this correlation separate from its own immutable `run_id` and
also includes it in the provider-neutral decision projection.

The command writes the complete `ingen.nublar-run/v1` record before returning
the run decision code:

```text
passed → 0
failed → 1
error  → 2
```

A failed producer result is a valid failed run. A missing required result,
malformed envelope, duplicate path, or producer mismatch is a Nublar-owned
collection error.

## Read a run

Use the immutable run ID to retrieve one record:

```sh
nublar run show --store .artifacts/nublar-runs --run-id <run-id>
```

`run show` returns the stored run's decision code. History can be listed and
filtered without changing records:

```sh
nublar run list --store .artifacts/nublar-runs \
  --status failed --workflow document-pipeline-ci
```

External attempts can be found with the same exact-match filters:

```sh
nublar run list --store .artifacts/nublar-runs \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
```

`run list` returns `0` when the read/export succeeds, regardless of individual
run statuses. Results are newest first, with ascending `run_id` as the tie
breaker.

## Export or deliver a decision

The provider-neutral projection excludes producer-owned reports:

```sh
nublar run decision \
  --store .artifacts/nublar-runs \
  --run-id <run-id> \
  --output decision.json
```

The command returns `0` when the projection is exported successfully; the
decision's `status` and `exit_code` remain in the JSON.

The reference delivery transport is an optional generic webhook:

```sh
nublar run deliver \
  --store .artifacts/nublar-runs \
  --run-id <run-id> \
  --webhook https://example.test/nublar \
  --receipt delivery-receipt.json \
  --receipt-store .artifacts/nublar-receipts
```

Delivery uses the run ID as its idempotency key. A delivery failure produces a
separate `ingen.nublar-delivery-receipt/v1` failure record and does not mutate
the stored run decision. Use `run receipt list --receipt-store <dir>` to read
the immutable local delivery-attempt history; add `--run-id`, `--status`, or
`--transport` for exact-match filters. See
[`RECEIPT-STORAGE.md`](RECEIPT-STORAGE.md) for its storage boundary.

## Identity and future extensions

`run_id` identifies one Nublar collection attempt. It is not a workflow ID,
commit ID, pull request ID, or external correlation ID. When supplied,
`correlation.system`, `correlation.id`, and `correlation.attempt` identify the
external attempt that requested the run. Recollecting the same workflow creates
a new run ID, and the filesystem store rejects reuse of an existing ID.

Rerun relationships, hosted APIs, retention, and provider-specific delivery
should be added only when a concrete consumer defines their required semantics.
They should not be encoded into `run_id`.

## Acceptance check

The Nublar-only verification gate is:

```sh
make nublar-check
```

It runs Nublar's race tests, static analysis, and schema syntax checks without
running any producer workflow.
