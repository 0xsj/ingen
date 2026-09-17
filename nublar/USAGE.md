# Nublar usage

This is the start-here guide for using Nublar in a local workspace or CI job.
Nublar is ready for live use within this scope: it consumes producer-written
`ingen.ci-result/v1` envelopes, records an immutable local run, produces a
provider-neutral decision, and can publish a GitHub Check or generic webhook.

Nublar does not launch Sorna, Paddock, Sentinel, or other producers. An
external workflow must create the declared result files first.

For the deeper contract and implementation notes, see
[`USAGE-GUIDE.md`](USAGE-GUIDE.md), [`STATUS.md`](STATUS.md), and
[`PRODUCT-BOUNDARY.md`](PRODUCT-BOUNDARY.md).

## 1. Prepare the workspace

Run these commands from the repository root. You need Go, Bash, `jq`, and the
producer tooling required by your workflow. The current CI workflow uses the
Go version declared in `go.mod`.

Create a private artifact workspace and build the CLI:

```sh
mkdir -p .artifacts/bin
go build -o .artifacts/bin/nublar ./nublar/cmd/nublar
NUBLAR_BIN="$PWD/.artifacts/bin/nublar"
```

Use the built binary when the caller must distinguish Nublar's exact exit
codes. `go run` can collapse a child exit code of `2` into its own status `1`.

## 2. Validate the workflow declaration

A workflow declares the producer envelopes that Nublar expects:

```sh
"$NUBLAR_BIN" workflow validate nublar/workflows/document-pipeline.yaml
```

The declaration's `result` paths are relative to the artifact root. Required
checks are the default; use `required: false` for an optional check. Nublar
rejects absolute paths, path escapes, duplicate paths, unknown fields, and
multiple YAML documents.

## 3. Produce the result envelopes

Run the producer workflow outside Nublar. Each producer must write a complete
`ingen.ci-result/v1` JSON envelope at the exact path declared by the workflow.
The artifact root should look like this before collection:

```text
.artifacts/
├── document-pipeline-go-provider-ci-result.json
├── document-pipeline-go-preparation-ci-result.json
├── document-pipeline-ci-result.json
└── document-pipeline-go-campaign-ci-result.json
```

Do not rename, merge, or rewrite producer envelopes for Nublar. Nublar
validates and preserves their exact bytes and SHA-256 references.

For a local smoke test without running producers, use the checked-in fixtures:

```sh
mkdir -p .artifacts/fixture-runs
set +e
"$NUBLAR_BIN" run collect \
  --workflow nublar/testdata/workflows/passed-producer.yaml \
  --root nublar/testdata/ci-results \
  --store .artifacts/fixture-runs \
  --output .artifacts/fixture-run.json \
  --run-id usage-passed-01
fixture_code=$?
set -e
test "$fixture_code" -eq 0
```

## 4. Collect and persist a run

Collect the external producer output into a run record and local run store:

```sh
mkdir -p .artifacts/nublar-runs
set +e
"$NUBLAR_BIN" run collect \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json \
  --external-system github-actions \
  --external-id "$GITHUB_RUN_ID" \
  --attempt "$GITHUB_RUN_ATTEMPT"
nublar_code=$?
set -e
printf 'Nublar decision exit code: %s\n' "$nublar_code"
```

The external correlation flags are optional, but must be supplied together.
They identify the external CI attempt; they do not replace Nublar's opaque
local `run_id`.

Nublar always uses this decision mapping:

| Status | Exit code | Meaning |
| --- | ---: | --- |
| `passed` | `0` | All present required checks passed. |
| `failed` | `1` | A producer reported a failed check. |
| `error` | `2` | Collection or validation could not produce a usable decision. |

The run is validated, hashed, and persisted before the decision exit code is
returned. Missing required artifacts, malformed envelopes, and producer/tool
mismatches become `error/2`. Missing optional artifacts remain warnings.

## 5. Inspect the run and history

Read the local run ID and inspect the complete stored record:

```sh
run_id="$(jq -er '.run_id' .artifacts/nublar-run.json)"
"$NUBLAR_BIN" run show \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --output .artifacts/nublar-run-shown.json
```

`run show` returns the stored run's decision code, so capture its status when
inspecting a failed or error run.

List history with optional exact filters:

```sh
"$NUBLAR_BIN" run list \
  --store .artifacts/nublar-runs \
  --status failed \
  --workflow document-pipeline-ci \
  --external-system github-actions \
  --external-id "$GITHUB_RUN_ID" \
  --attempt "$GITHUB_RUN_ATTEMPT"
```

`run list` is a read operation: it returns `0` when the store was read and the
list was written successfully, even if the listed runs are failed or in error.
History is deterministic and newest first.

## 6. Export the delivery decision

Export the compact provider-neutral projection before delivering it:

```sh
"$NUBLAR_BIN" run decision \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --output .artifacts/nublar-decision.json
```

The decision includes run identity, workflow and result references, check
statuses, warnings, errors, timestamps, and correlation. It excludes nested
producer reports and explanations. Export success returns `0`; the decision's
own `status` and `exit_code` fields carry the run outcome.

## 7. Deliver to a destination

Delivery is explicit and does not change the stored run decision. Every
publisher attempt can write a separate receipt.

### Generic HTTP webhook

```sh
"$NUBLAR_BIN" run deliver \
  --transport http-webhook \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --webhook https://example.test/nublar \
  --receipt .artifacts/nublar-webhook-receipt.json \
  --receipt-store .artifacts/nublar-receipts
```

The webhook receives the provider-neutral decision and the `run_id` as its
`Idempotency-Key`. Optional HMAC signing reads a secret from the environment:

```sh
"$NUBLAR_BIN" run deliver \
  --transport http-webhook \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --webhook "$NUBLAR_WEBHOOK_URL" \
  --secret-env NUBLAR_WEBHOOK_SECRET \
  --receipt-store .artifacts/nublar-receipts
```

### GitHub Check

Use the workflow token from the environment; do not put credentials in the
command line. The publishing job needs the least permission that can create
and update checks, normally `checks: write` plus any required read
permissions:

```sh
"$NUBLAR_BIN" run deliver \
  --transport github-checks \
  --store .artifacts/nublar-runs \
  --run-id "$run_id" \
  --repository "$GITHUB_REPOSITORY" \
  --head-sha "$GITHUB_SHA" \
  --details-url "${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}" \
  --token-env GITHUB_TOKEN \
  --receipt .artifacts/nublar-github-checks-receipt.json \
  --receipt-store .artifacts/nublar-receipts
```

The check name defaults to `Nublar / <workflow-id>`. Nublar `passed` maps to a
GitHub `success` check. Nublar `failed` and `error` map to a failing check,
while the exact Nublar status and exit code remain in the check output.
Repeated delivery of one local `run_id` updates the same remote check.

For a complete live example, use the manual workflow:
[`../.github/workflows/nublar-consumer.yml`](../.github/workflows/nublar-consumer.yml).

## 8. Read delivery receipts

Receipts are audit records, not retry queues:

```sh
"$NUBLAR_BIN" run receipt list \
  --receipt-store .artifacts/nublar-receipts \
  --run-id "$run_id" \
  --status accepted \
  --transport github-checks \
  --output .artifacts/nublar-receipts.json
```

Receipt status describes delivery, not the Nublar decision. A failed delivery
does not modify the stored run or decision. Repeating delivery keeps one run
identity and records independent receipt outcomes.

## 9. Use the CI gate helper

For a shell-based CI integration, use the provider-neutral helper. It derives
the final decision from the persisted run so the `0`, `1`, and `2` contract is
preserved even when invoked through `go run`:

```sh
set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json \
  --external-system github-actions \
  --external-id "$GITHUB_RUN_ID" \
  --attempt "$GITHUB_RUN_ATTEMPT"
nublar_code=$?
set -e
exit "$nublar_code"
```

The helper writes the run and decision artifacts and returns `0` for passed,
`1` for failed, and `2` for collection or export errors.

## 10. Compatibility aggregate

Existing one-shot users can continue to use the compatibility command:

```sh
"$NUBLAR_BIN" aggregate \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --output .artifacts/nublar-result.json
```

New integrations should use `run collect`, which adds immutable run identity,
history, correlation, decisions, delivery, and receipts. The aggregate output
shape remains the older `ingen.nublar-result/v1` contract.

## 11. Verify the installation and contract

From the repository root:

```sh
make nublar-freeze-check
```

This runs race tests, static analysis, schema checks, strict-ingestion cases,
consumer scenarios, history and receipt assertions, and whitespace validation.

## 12. Troubleshoot common failures

- `error/2` with a missing path: confirm the producer wrote the exact relative
  path declared in the workflow and that `--root` points to its parent.
- `error/2` with a producer mismatch: the envelope's `tool` must match the
  workflow check's declared `tool`.
- `error/2` with malformed JSON or unknown fields: regenerate the complete
  shared envelope; do not edit it into a different shape.
- Failed webhook or GitHub delivery: inspect the failed receipt; the stored
  run is intentionally unchanged.
- GitHub `403`: confirm the publishing job has `checks: write`; fork and
  Dependabot workflows may receive read-only tokens.
- Empty `run list` or receipt list: an empty JSON array is a successful read,
  not a storage failure.
- Need cross-runner history or retention beyond workflow artifacts: the local
  store is not intended for that; submit a consumer request before adding
  hosted storage.

## Live-use boundary

The current live-use boundary is one external producer workflow, one Nublar
consumer job, local filesystem history for that job, optional workflow
artifact upload, and optional destination delivery. The validated GitHub
Actions example uses separate producer and Nublar jobs and 14-day workflow
artifact retention. Hosted Nublar services, remote history, producer
scheduling, and additional provider integrations require a new concrete
consumer requirement.
