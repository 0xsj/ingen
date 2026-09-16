#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../../.." && pwd)"
cd -- "$repo_root"

if [[ -z "${GOCACHE:-}" ]]; then
  export GOCACHE="$repo_root/.cache/go-build"
fi
if [[ -z "${GOMODCACHE:-}" ]]; then
  export GOMODCACHE="$repo_root/.cache/go-mod"
fi

check_root="$(mktemp -d "${TMPDIR:-/tmp}/nublar-consumer-check.XXXXXX")"
trap 'rm -rf -- "$check_root"' EXIT

set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/mixed-producers.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/runs" \
  --output "$check_root/run.json" \
  --run-id nublar-consumer-check-01 \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
consumer_code=$?
set -e

if [[ "$consumer_code" -ne 1 ]]; then
  echo "consumer example exit code=$consumer_code, want 1 for fixture failure" >&2
  exit 1
fi

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-check-01" and
  .status == "failed" and
  .exit_code == 1 and
  .correlation == {system: "github-actions", id: "build-42", attempt: 3}
' "$check_root/run.json" >/dev/null

jq -e '
  .schema == "ingen.nublar-decision/v1" and
  .run_id == "nublar-consumer-check-01" and
  .status == "failed" and
  .exit_code == 1 and
  .correlation == {system: "github-actions", id: "build-42", attempt: 3} and
  ([.checks[] | has("artifact")] | any) == false
' "$check_root/run.decision.json" >/dev/null

echo "Nublar consumer example passed with expected decision exit code 1"

set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/passed-producer.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/passed-runs" \
  --output "$check_root/passed-run.json" \
  --run-id nublar-consumer-check-02 \
  --external-system github-actions \
  --external-id build-43 \
  --attempt 1
passed_code=$?
set -e

if [[ "$passed_code" -ne 0 ]]; then
  echo "consumer example exit code=$passed_code, want 0 for fixture success" >&2
  exit 1
fi

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-check-02" and
  .status == "passed" and
  .exit_code == 0 and
  .correlation == {system: "github-actions", id: "build-43", attempt: 1}
' "$check_root/passed-run.json" >/dev/null

jq -e '
  .schema == "ingen.nublar-decision/v1" and
  .run_id == "nublar-consumer-check-02" and
  .status == "passed" and
  .exit_code == 0 and
  .correlation == {system: "github-actions", id: "build-43", attempt: 1} and
  ([.checks[] | has("artifact")] | any) == false
' "$check_root/passed-run.decision.json" >/dev/null

set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/missing-producer.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/error-runs" \
  --output "$check_root/error-run.json" \
  --run-id nublar-consumer-check-03 \
  --external-system github-actions \
  --external-id build-44 \
  --attempt 1
error_code=$?
set -e

if [[ "$error_code" -ne 2 ]]; then
  echo "consumer example exit code=$error_code, want 2 for collection error" >&2
  exit 1
fi

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-check-03" and
  .status == "error" and
  .exit_code == 2 and
  (.errors | length) == 1 and
  .correlation == {system: "github-actions", id: "build-44", attempt: 1}
' "$check_root/error-run.json" >/dev/null

jq -e '
  .schema == "ingen.nublar-decision/v1" and
  .run_id == "nublar-consumer-check-03" and
  .status == "error" and
  .exit_code == 2 and
  .correlation == {system: "github-actions", id: "build-44", attempt: 1} and
  ([.checks[] | has("artifact")] | any) == false
' "$check_root/error-run.decision.json" >/dev/null

set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/malformed-producer.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/malformed-runs" \
  --output "$check_root/malformed-run.json" \
  --run-id nublar-consumer-check-04 \
  --external-system github-actions \
  --external-id build-45 \
  --attempt 1
malformed_code=$?
set -e

if [[ "$malformed_code" -ne 2 ]]; then
  echo "consumer example exit code=$malformed_code, want 2 for malformed envelope" >&2
  exit 1
fi

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-check-04" and
  .status == "error" and
  .exit_code == 2 and
  .checks[0].status == "error" and
  (.checks[0].reason | contains("unknown field")) and
  .correlation == {system: "github-actions", id: "build-45", attempt: 1}
' "$check_root/malformed-run.json" >/dev/null

jq -e '
  .schema == "ingen.nublar-decision/v1" and
  .run_id == "nublar-consumer-check-04" and
  .status == "error" and
  .exit_code == 2 and
  .correlation == {system: "github-actions", id: "build-45", attempt: 1} and
  ([.checks[] | has("artifact")] | any) == false
' "$check_root/malformed-run.decision.json" >/dev/null

set +e
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/mixed-producers.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/history-runs" \
  --output "$check_root/history-failed-run.json" \
  --run-id nublar-consumer-history-failed \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
history_failed_code=$?

bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/testdata/workflows/passed-producer.yaml \
  --root nublar/testdata/ci-results \
  --store "$check_root/history-runs" \
  --output "$check_root/history-passed-run.json" \
  --run-id nublar-consumer-history-passed \
  --external-system github-actions \
  --external-id build-43 \
  --attempt 1
history_passed_code=$?
set -e

if [[ "$history_failed_code" -ne 1 || "$history_passed_code" -ne 0 ]]; then
  echo "history setup codes=$history_failed_code/$history_passed_code, want 1/0" >&2
  exit 1
fi

go run ./nublar/cmd/nublar run list \
  --store "$check_root/history-runs" \
  --status failed \
  --workflow mixed-producers-ci \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3 \
  --output "$check_root/history-filtered.json"

jq -e '
  length == 1 and
  .[0].run_id == "nublar-consumer-history-failed" and
  .[0].status == "failed" and
  .[0].correlation == {system: "github-actions", id: "build-42", attempt: 3}
' "$check_root/history-filtered.json" >/dev/null

go run ./nublar/cmd/nublar run show \
  --store "$check_root/history-runs" \
  --run-id nublar-consumer-history-passed \
  --output "$check_root/history-shown.json"

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-history-passed" and
  .status == "passed" and
  (.checks | length) == 1
' "$check_root/history-shown.json" >/dev/null

set +e
go run ./nublar/cmd/nublar run show \
  --store "$check_root/history-runs" \
  --run-id nublar-consumer-history-failed \
  --output "$check_root/history-failed-shown.json"
failed_show_code=$?
set -e

if [[ "$failed_show_code" -ne 1 ]]; then
  echo "run show exit code=$failed_show_code, want stored failed decision code 1" >&2
  exit 1
fi

jq -e '
  .schema == "ingen.nublar-run/v1" and
  .run_id == "nublar-consumer-history-failed" and
  .status == "failed" and
  .exit_code == 1
' "$check_root/history-failed-shown.json" >/dev/null

echo "Nublar consumer example passed decision, strict-ingestion, and history paths"
