# Nublar usage guide

Nublar consumes producer-written `ingen.ci-result/v1` envelopes, records one
immutable collection attempt, and exposes a provider-neutral decision for CI
or delivery systems. It does not launch Sorna, Paddock, or another producer.

This guide covers the current local filesystem-backed slice. Start with the
run-oriented path for new integrations; `aggregate` is retained for the older
compatibility output.

## 1. Build the CLI

From the repository root:

```sh
mkdir -p .artifacts/bin
go build -o .artifacts/bin/nublar ./nublar/cmd/nublar
NUBLAR_BIN=.artifacts/bin/nublar
```

Use a built binary when the caller must distinguish Nublar's exact exit codes.
The `go run` wrapper can report a program exit code of `2` as its own status
`1`; the bundled consumer example reads the persisted run code to preserve the
contract.

## 2. Validate the workflow

A workflow names the result files that a producer workflow is expected to
write. Checks are required by default; set `required: false` for an optional
result.

```sh
"$NUBLAR_BIN" workflow validate nublar/workflows/document-pipeline.yaml
```

Result paths are relative to the artifact root passed to `--root`. Nublar
rejects absolute paths, path escapes, duplicate result paths, unknown workflow
fields, and multiple YAML documents.

## 3. Collect and persist a run

The producer workflow must write complete CI envelopes before collection:

```sh
mkdir -p .artifacts/nublar-runs
set +e
"$NUBLAR_BIN" run collect \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
collect_code=$?
set -e
printf 'nublar collect exit code: %s\n' "$collect_code"
```

Collection validates every declared envelope, records the exact workflow and
result-file hashes, preserves producer artifacts, and writes the run before
returning its decision code:

| Run status | Exit code | Meaning |
| --- | ---: | --- |
| `passed` | `0` | All present checks passed. |
| `failed` | `1` | A producer reported a failed check. |
| `error` | `2` | Collection or validation could not produce a valid decision. |

The `--external-system`, `--external-id`, and `--attempt` flags are optional,
but must be supplied as a complete tuple. They identify the external CI
attempt; they do not replace the local immutable `run_id`.

For a self-contained successful example, use the checked-in fixture:

```sh
set +e
"$NUBLAR_BIN" run collect \
  --workflow nublar/testdata/workflows/passed-producer.yaml \
  --root nublar/testdata/ci-results \
  --store .artifacts/example-runs \
  --output .artifacts/example-run.json \
  --run-id usage-guide-passed-01
collect_code=$?
set -e
test "$collect_code" -eq 0
```

The mixed-producer fixture demonstrates the failed path, while the missing and
malformed fixtures demonstrate collection errors:

```text
nublar/testdata/workflows/mixed-producers.yaml       → failed / 1
nublar/testdata/workflows/missing-producer.yaml      → error / 2
nublar/testdata/workflows/malformed-producer.yaml    → error / 2
```

## 4. Inspect runs

Read the generated ID from the run artifact or use an ID supplied by the
caller:

```sh
run_id="$(jq -r '.run_id' .artifacts/nublar-run.json)"
"$NUBLAR_BIN" run show \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --output .artifacts/nublar-run-shown.json
```

`run show` writes the validated run and returns that run's stored decision
code. Capture its status when showing a failed or error run.

List history without changing stored records:

```sh
"$NUBLAR_BIN" run list \
  --store .artifacts/nublar-runs \
  --status failed \
  --workflow document-pipeline-ci
```

Run history is newest first, with ascending `run_id` as the deterministic tie
breaker. Status, workflow, and external-correlation filters can be combined.
`run list` returns `0` when the read and JSON export succeed, even if matching
runs are failed or in error.

## 5. Export the delivery decision

The decision is the compact provider-neutral projection. It retains run and
check references, status, issues, timestamps, and correlation, but excludes
producer-owned reports and explanations:

```sh
"$NUBLAR_BIN" run decision \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --output .artifacts/nublar-decision.json
```

Projection export returns `0` when the JSON is written successfully. The
decision's own `status` and `exit_code` fields carry the run outcome.

## 6. Deliver and preserve a receipt

Delivery is explicit. The reference transport posts the decision to a generic
HTTP webhook and uses `run_id` as the `Idempotency-Key`:

```sh
"$NUBLAR_BIN" run deliver \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --webhook https://example.test/nublar \
  --receipt .artifacts/nublar-receipt.json \
  --receipt-store .artifacts/nublar-receipts
```

Nublar contacts a network endpoint only when `run deliver` is invoked. A
publisher-reached failure produces a separate failed receipt and does not
change the stored run decision. Optional HMAC signing reads its secret from an
environment variable:

```sh
# NUBLAR_WEBHOOK_SECRET must be injected by the environment or CI secret store.
"$NUBLAR_BIN" run deliver \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --webhook https://example.test/nublar \
  --secret-env NUBLAR_WEBHOOK_SECRET \
  --receipt-store .artifacts/nublar-receipts
```

Read receipt history with exact optional filters:

```sh
"$NUBLAR_BIN" run receipt list \
  --receipt-store .artifacts/nublar-receipts \
  --run-id "$run_id" \
  --status failed \
  --transport http-webhook \
  --output .artifacts/nublar-receipts.json
```

Receipts are audit records, not retries. Repeating delivery keeps the same run
identity and creates a separate receipt for each publisher attempt.

## 7. Compatibility aggregate

The older command remains available for consumers that need one aggregate
result without a persisted run:

```sh
"$NUBLAR_BIN" aggregate \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --output .artifacts/nublar-result.json
```

Use `run collect` for the current persisted lifecycle. `aggregate` does not
create run history or delivery receipts.

## 8. Verify the local slice

Run the complete local checkpoint from the repository root:

```sh
make nublar-freeze-check
```

This runs Nublar's race tests, static analysis, schema checks, consumer
scenarios, history and receipt assertions, and diff-whitespace validation.

## Troubleshooting

- Missing required result: the run is persisted as `error/2` with a Nublar-owned issue.
- Missing optional result: the run remains valid and records a warning.
- Malformed or unknown-field envelope: the check becomes a collection error; producer data is not accepted.
- Producer/tool mismatch: the check becomes a collection error.
- Failed delivery: the receipt records the attempt; the stored run is unchanged.
- Empty history: `run list` and `run receipt list` return an empty JSON array.
- Exact status handling: use a built CLI binary when process-level `0`, `1`, and `2` must be distinguished.

The current boundaries and deferred capabilities are documented in
[`PRODUCT-BOUNDARY.md`](PRODUCT-BOUNDARY.md), [`DELIVERY-BOUNDARY.md`](DELIVERY-BOUNDARY.md),
[`RUN-QUERY.md`](RUN-QUERY.md), and [`FREEZE-RECORD.md`](FREEZE-RECORD.md).
