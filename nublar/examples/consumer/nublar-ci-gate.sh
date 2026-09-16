#!/usr/bin/env bash

set -u -o pipefail

usage() {
  cat >&2 <<'USAGE'
usage: nublar-ci-gate.sh --workflow <path> [options]

Collects already-produced CI envelopes, persists the run, exports its
provider-neutral decision, and returns the Nublar decision exit code.

options:
  --root <dir>                 artifact root (default: .artifacts)
  --store <dir>                run store (default: .artifacts/nublar-runs)
  --output <path>              run output (default: .artifacts/nublar-run.json)
  --run-id <id>                optional local run ID
  --external-system <name>     optional external correlation system
  --external-id <id>           optional external correlation ID
  --attempt <n>                optional external correlation attempt
USAGE
}

workflow=""
root=".artifacts"
store=".artifacts/nublar-runs"
output=".artifacts/nublar-run.json"
run_id=""
external_system=""
external_id=""
attempt=""

while (($# > 0)); do
  case "$1" in
    --workflow|--root|--store|--output|--run-id|--external-system|--external-id|--attempt)
      if (($# < 2)); then
        echo "missing value for $1" >&2
        usage
        exit 2
      fi
      case "$1" in
        --workflow) workflow="$2" ;;
        --root) root="$2" ;;
        --store) store="$2" ;;
        --output) output="$2" ;;
        --run-id) run_id="$2" ;;
        --external-system) external_system="$2" ;;
        --external-id) external_id="$2" ;;
        --attempt) attempt="$2" ;;
      esac
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$workflow" ]]; then
  echo "--workflow is required" >&2
  usage
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../../.." && pwd)"
cd -- "$repo_root" || exit 2

if [[ -z "${GOCACHE:-}" ]]; then
  export GOCACHE="$repo_root/.cache/go-build"
fi
if [[ -z "${GOMODCACHE:-}" ]]; then
  export GOMODCACHE="$repo_root/.cache/go-mod"
fi

output_dir="$(dirname -- "$output")"
mkdir -p -- "$output_dir" || {
  echo "cannot create output directory: $output_dir" >&2
  exit 2
}

collect_args=(
  run collect
  --workflow "$workflow"
  --root "$root"
  --store "$store"
  --output "$output"
)
[[ -n "$run_id" ]] && collect_args+=(--run-id "$run_id")
[[ -n "$external_system" ]] && collect_args+=(--external-system "$external_system")
[[ -n "$external_id" ]] && collect_args+=(--external-id "$external_id")
[[ -n "$attempt" ]] && collect_args+=(--attempt "$attempt")

set +e
go run ./nublar/cmd/nublar "${collect_args[@]}"
collect_code=$?
set -e

if [[ ! -s "$output" ]]; then
  echo "Nublar did not publish a run artifact at $output" >&2
  exit 2
fi

run_id_from_output="$(jq -er '.run_id' "$output")" || {
  echo "Nublar run artifact has no readable run_id: $output" >&2
  exit 2
}
status_from_output="$(jq -er '.status' "$output")" || {
  echo "Nublar run artifact has no readable status: $output" >&2
  exit 2
}
decision_code_from_output="$(jq -er '.exit_code' "$output")" || {
  echo "Nublar run artifact has no readable exit_code: $output" >&2
  exit 2
}
case "$decision_code_from_output" in
  0|1|2) ;;
  *)
    echo "Nublar run artifact has unsupported exit_code $decision_code_from_output: $output" >&2
    exit 2
    ;;
esac
expected_collect_process_code=0
if [[ "$decision_code_from_output" -ne 0 ]]; then
  expected_collect_process_code=1
fi
if [[ "$collect_code" -ne "$expected_collect_process_code" ]]; then
  echo "Nublar collect process code=$collect_code disagrees with persisted exit_code=$decision_code_from_output" >&2
  exit 2
fi

if [[ "$output" == *.json ]]; then
  decision_output="${output%.json}.decision.json"
else
  decision_output="${output}.decision.json"
fi
if ! go run ./nublar/cmd/nublar run decision \
  --store "$store" \
  --run-id "$run_id_from_output" \
  --output "$decision_output"; then
  echo "Nublar could not export the provider-neutral decision" >&2
  exit 2
fi

printf 'nublar_run_id=%s\n' "$run_id_from_output"
printf 'nublar_status=%s\n' "$status_from_output"
printf 'nublar_exit_code=%s\n' "$decision_code_from_output"
printf 'nublar_run=%s\n' "$output"
printf 'nublar_decision=%s\n' "$decision_output"
exit "$decision_code_from_output"
