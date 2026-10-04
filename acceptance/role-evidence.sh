#!/usr/bin/env bash
# Exercise actual contained commands and their producer/consumer handoffs.
# This is an execution-evidence check, not contract approval or subject verification.
set -euo pipefail

if [[ "$(uname -s)" != Darwin ]]; then
  echo 'This acceptance check requires macOS Seatbelt.' >&2
  exit 2
fi
checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
proof="$(mktemp -d /private/tmp/ingen-role-evidence.XXXXXX)"
trap 'printf "evidence root: %s\n" "$proof"' EXIT
export GOCACHE="${GOCACHE:-$checkout/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$checkout/.cache/go-mod}"
mkdir -p "$proof/bin"
cd "$checkout"
for tool in sentinel lockwood nublar sattler; do
  module="$tool"
  [[ "$tool" != sentinel ]] || module=herdr-sentinel
  go build -o "$proof/bin/$tool" "./$module/cmd/$tool"
done

expect_exit() {
  local wanted="$1" actual=0
  shift
  "$@" || actual=$?
  if [[ "$actual" != "$wanted" ]]; then
    printf 'expected exit %s, got %s: %s\n' "$wanted" "$actual" "$*" >&2
    exit 1
  fi
}

for outcome in passed failed; do
  project="$proof/$outcome"
  mkdir -p "$project"
  "$proof/bin/sentinel" project init --root "$project" --id role-evidence
  "$proof/bin/sentinel" run bootstrap --root "$project" \
    --workspace .ingen/workspace.yaml --output "$project/.ingen/artifacts/receipt.json"
  command=(/bin/echo role-evidence-success)
  expected=0
  command_exit=0
  if [[ "$outcome" == failed ]]; then
    command=(/bin/bash -c 'exit 7')
    expected=1
    command_exit=7
  fi
  expect_exit "$command_exit" "$proof/bin/sentinel" session spawn --root "$project" \
    --workspace .ingen/workspace.yaml --receipt .ingen/artifacts/receipt.json \
    --role implementation --isolate -- "${command[@]}"
  reports=()
  for candidate in "$project"/.ingen/artifacts/role-executions/*.json; do
    [[ "$candidate" == *.policy.json ]] || reports+=("$candidate")
  done
  [[ "${#reports[@]}" == 1 ]]
  report="${reports[0]#$project/}"
  digest="$(shasum -a 256 "$project/$report" | cut -d ' ' -f 1)"
  expect_exit "$expected" "$proof/bin/sentinel" role verify --root "$project" \
    --path "$report" --expected-sha256 "$digest" --ci-result .ingen/artifacts/role-ci.json
  jq -e --arg outcome "$outcome" --argjson command_exit "$command_exit" \
    '.tool == "sentinel" and .kind == "role-execution" and .status == $outcome and .report.exit_code == $command_exit' \
    "$project/.ingen/artifacts/role-ci.json" >/dev/null
  "$proof/bin/lockwood" import-role-execution --root "$project/.ingen/artifacts/custody" \
    --project-root "$project" --path "$report" --expected-digest "$digest" --id role-evidence
  "$proof/bin/lockwood" verify --root "$project/.ingen/artifacts/custody" --id role-evidence
  "$proof/bin/lockwood" get --root "$project/.ingen/artifacts/custody" \
    --output "$project/.ingen/artifacts/retrieved-report.json" "sha256:$digest"
  cmp "$project/$report" "$project/.ingen/artifacts/retrieved-report.json"
  cat > "$project/.ingen/nublar/role-evidence.yaml" <<'YAML'
schema: ingen.nublar-workflow/v1
id: contained-role-evidence
checks:
  - id: contained-command
    tool: sentinel
    result: .ingen/artifacts/role-ci.json
    required: true
YAML
  expect_exit "$expected" "$proof/bin/nublar" run collect \
    --workflow "$project/.ingen/nublar/role-evidence.yaml" --root "$project" \
    --store "$project/.ingen/artifacts/nublar-runs" --output "$project/.ingen/artifacts/nublar-run.json"
  # Intake retains original bytes; damage to a capture must block fresh verification/intake.
  capture="$(jq -r '.stdout_path' "$project/$report")"
  cp "$project/$capture" "$project/$capture.original"
  printf 'tampered capture\n' >> "$project/$capture"
  expect_exit 1 "$proof/bin/sentinel" role verify --root "$project" --path "$report" --expected-sha256 "$digest"
  expect_exit 1 "$proof/bin/lockwood" import-role-execution --root "$project/.ingen/artifacts/custody" \
    --project-root "$project" --path "$report" --expected-digest "$digest" --id tampered-evidence
  # Custody still verifies its preserved original bytes, independently of damaged source files.
  "$proof/bin/lockwood" verify --root "$project/.ingen/artifacts/custody" --id role-evidence
  mv "$project/$capture" "$project/$capture.tampered"
  mv "$project/$capture.original" "$project/$capture"
done

"$proof/bin/sattler" compare --format json --output "$proof/comparison.json" \
  "$proof/passed/.ingen/artifacts/role-ci.json" "$proof/failed/.ingen/artifacts/role-ci.json"
jq -e '.compatible and .before.status == "passed" and .after.status == "failed"' "$proof/comparison.json" >/dev/null
printf 'Contained success/failure, required CI checks, custody, comparison, and source tamper rejection passed.\n'
