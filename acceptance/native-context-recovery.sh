#!/usr/bin/env bash
# Offline typed-agent proof by default; host interaction requires explicit opt-in.
set -euo pipefail

usage() {
  echo "usage: $0 [--live --socket CANONICAL_ABSOLUTE_HERDR_SOCKET]" >&2
}

live=false
socket_path=""
while (($#)); do
  case "$1" in
    --live) live=true; shift ;;
    --socket) (($# >= 2)) || { usage; exit 2; }; socket_path="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture="$checkout/acceptance/native-context-recovery"
if [[ "$(uname -s)" != Darwin ]]; then
  echo 'This acceptance check requires macOS Seatbelt.' >&2
  exit 2
fi
if [[ "$live" == true ]]; then
  if [[ -z "$socket_path" || "$socket_path" != /* || ! -S "$socket_path" || -L "$socket_path" || "${HERDR_ENV:-}" != 1 ]]; then
    echo 'Live mode requires HERDR_ENV=1 and an explicit canonical absolute socket path.' >&2
    exit 2
  fi
  socket_parent="$(cd "$(dirname "$socket_path")" && pwd -P)"
  if [[ "$socket_path" != "$socket_parent/$(basename "$socket_path")" ]]; then
    echo 'Live mode requires a canonical socket path (resolve aliases before selecting it).' >&2
    exit 2
  fi
elif [[ -n "$socket_path" ]]; then
  echo '--socket is only valid with --live.' >&2
  exit 2
fi

proof="$(mktemp -d /private/tmp/ingen-native-context.XXXXXX)"
receiver_pid=""
live_project=""
live_projects=()
live_closed_journals=()
cleanup() {
  if [[ "$live" == true ]]; then
    for cleanup_project in "${live_projects[@]:-}"; do
      [[ -d "$cleanup_project/.ingen/artifacts/native-sessions" ]] || continue
      for journal_file in "$cleanup_project"/.ingen/artifacts/native-sessions/*.json; do
        [[ -f "$journal_file" ]] || continue
        journal_rel="${journal_file#"$cleanup_project"/}"
        case " ${live_closed_journals[*]:-} " in *" $cleanup_project:$journal_rel "*) continue ;; esac
        journal_state="$(jq -r '.state' "$journal_file" 2>/dev/null || true)"
        case "$journal_state" in
          submitted|running|cancel-requested)
            HERDR_ENV=1 "$proof/bin/sentinel" session native-cancel --socket "$socket_path" \
              --root "$cleanup_project" --path "$journal_rel" >/dev/null 2>&1 || true
            for _ in $(seq 1 60); do
              journal_state="$(jq -r '.state' "$journal_file" 2>/dev/null || true)"
              case "$journal_state" in completed|failed|canceled|indeterminate) break ;; esac
              sleep 0.1
            done
            ;;
        esac
        case "$journal_state" in
          completed|failed|canceled)
            HERDR_ENV=1 "$proof/bin/sentinel" session native-close --socket "$socket_path" \
              --root "$cleanup_project" --path "$journal_rel" >/dev/null 2>&1 || \
              echo "cleanup could not confirm safe close for owned journal $journal_rel; inspect evidence root" >&2
            ;;
          indeterminate|*)
            echo "cleanup left owned workspace open because journal $journal_rel is $journal_state; inspect evidence root" >&2
            ;;
        esac
      done
    done
  fi
  if [[ -n "$receiver_pid" ]]; then
    if kill -0 "$receiver_pid" 2>/dev/null; then
      kill "$receiver_pid" 2>/dev/null || true
      wait "$receiver_pid" 2>/dev/null || true
    fi
    receiver_pid=""
  fi
  printf 'evidence root: %s\n' "$proof"
}
trap cleanup EXIT
export GOCACHE="${GOCACHE:-$checkout/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$checkout/.cache/go-mod}"
mkdir -p "$proof/bin"
cd "$checkout"
go build -o "$proof/bin/sentinel" ./herdr-sentinel/cmd/sentinel
go build -o "$proof/bin/codex-fixture" ./acceptance/native-context-recovery/codex-fixture
go build -o "$proof/bin/lockwood" ./lockwood/cmd/lockwood
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go test -count=1 -v ./herdr-sentinel/internal/nativesession >"$proof/native-session-tests.log" 2>&1
grep -q '^--- PASS: TestRecoverAfterWrapperCrashReleasesExecutionLease ' "$proof/native-session-tests.log"

new_project() {
  local project="$1" id="$2"
  mkdir -p "$project"
  "$proof/bin/sentinel" project init --root "$project" --id "$id" >"$proof/$id-init.log"
  python3 - "$project/.ingen/workspace.yaml" <<'PY'
from pathlib import Path
import re
import sys
path = Path(sys.argv[1])
data = path.read_text()
roles = list(re.finditer(r"(?m)^( +)- id: implementation\n", data))
if len(roles) != 1:
    raise SystemExit("expected exactly one generated implementation role")
role = roles[0]
following = re.search(r"(?m)^" + re.escape(role.group(1)) + r"- id:", data[role.end():])
end = role.end() + following.start() if following else len(data)
block = data[role.start():end]
block, count = re.subn(r"(?m)^( +)- \.ingen/contract/contract\.json$", r"\1- .ingen/contract", block)
if count != 1:
    raise SystemExit("expected one implementation contract read grant")
block, count = re.subn(r"(?m)^( +)- \.git$", r"\1- .ingen/legacy-context\n\1- .git", block)
if count != 1:
    raise SystemExit("expected one implementation .git deny grant")
path.write_text(data[:role.start()] + block + data[end:])
PY
  "$proof/bin/sentinel" workspace validate "$project/.ingen/workspace.yaml" >"$proof/$id-workspace.log"
  "$proof/bin/sentinel" run bootstrap --root "$project" --workspace .ingen/workspace.yaml \
    --output "$project/.ingen/artifacts/run.json" >"$proof/$id-bootstrap.log"
  mkdir -p "$project/.ingen/legacy-context/home/.codex" \
    "$project/.ingen/legacy-context/codex" "$project/.ingen/artifacts/role-executions"
  printf 'prior home session data\n' >"$project/.ingen/legacy-context/home/.codex/session.json"
  printf 'prior Codex history\n' >"$project/.ingen/legacy-context/codex/history.jsonl"
  printf 'private prior context\n' >"$project/.ingen/legacy-context/private.json"
}

wait_for_file() {
  local path="$1" tries=0
  while [[ ! -s "$path" && "$tries" -lt 100 ]]; do
    sleep 0.05
    tries=$((tries + 1))
  done
  [[ -s "$path" ]]
}

receiver_address_file="$proof/receiver.address"
receiver_accepted_file="$proof/receiver.accepted"
python3 "$fixture/loopback_receiver.py" --address-file "$receiver_address_file" \
  --accepted-file "$receiver_accepted_file" >"$proof/receiver.log" 2>&1 &
receiver_pid=$!
wait_for_file "$receiver_address_file" || { cat "$proof/receiver.log" >&2; exit 1; }
probe_address="$(cat "$receiver_address_file")"
python3 - "$probe_address" <<'PY'
import socket, sys
host, port = sys.argv[1].rsplit(":", 1)
with socket.create_connection((host, int(port)), timeout=2) as connection:
    connection.sendall(b"host-control-probe")
PY
wait_for_file "$receiver_accepted_file" || { echo 'loopback control target did not accept its positive host probe' >&2; exit 1; }
grep -qx 'host-control-probe' "$receiver_accepted_file"

offline_project="$proof/offline"
new_project "$offline_project" native-context-offline
cat >"$offline_project/.ingen/contract/offline.txt" <<EOF
NATIVE_FIXTURE_MODE=complete
FORBIDDEN_CONTEXT=$offline_project/.ingen/legacy-context/private.json
LEGACY_HOME=$offline_project/.ingen/legacy-context/home
LEGACY_CODEX_HOME=$offline_project/.ingen/legacy-context/codex
NETWORK_PROBE_ADDR=$probe_address
This is an offline fixture prompt. No model provider is contacted.
EOF
env OPENAI_API_KEY=fixture-only-not-a-secret "$proof/bin/sentinel" session spawn \
  --root "$offline_project" --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/run.json --role implementation --isolate \
  --agent codex --agent-executable "$proof/bin/codex-fixture" \
  --prompt-file .ingen/contract/offline.txt >"$proof/offline-spawn.log" 2>&1

report="$(python3 - "$offline_project" <<'PY'
import json, sys
from pathlib import Path
root = Path(sys.argv[1])
matches = []
for path in (root / ".ingen/artifacts/role-executions").glob("*.json"):
    if path.name.endswith(".policy.json"):
        continue
    try:
        value = json.loads(path.read_bytes())
    except Exception:
        continue
    if value.get("schema") == "ingen.sentinel-role-execution/v1":
        matches.append(path.relative_to(root).as_posix())
if len(matches) != 1:
    raise SystemExit(f"expected one role execution report, found {matches}")
print(matches[0])
PY
)"
report_file="$offline_project/$report"
jq -e '.status == "completed" and .agent_context.agent == "codex-cli" and .agent_context.mode == "exec-ephemeral" and .agent_context.stdin == "hash-pinned-prompt-snapshot" and .agent_context.network_mode == "disabled" and (.agent_context.home_path | contains("/.runtime/")) and (.agent_context.codex_home_path | contains("/.runtime/"))' "$report_file" >/dev/null
source_path="$(jq -r '.agent_context.prompt_source_path' "$report_file")"
snapshot_path="$(jq -r '.agent_context.prompt_snapshot_path' "$report_file")"
cmp "$offline_project/$source_path" "$offline_project/$snapshot_path"
home_path="$(jq -r '.agent_context.home_path' "$report_file")"
cmp "$offline_project/$source_path" "$home_path/prompt.received"
grep -q '"prior_context_unreadable":true' "$offline_project/$(jq -r '.stdout_path' "$report_file")"
grep -q '"credential_free":true' "$offline_project/$(jq -r '.stdout_path' "$report_file")"
grep -q '"network_denied":true' "$offline_project/$(jq -r '.stdout_path' "$report_file")"
[[ "$(wc -l <"$receiver_accepted_file" | tr -d ' ')" == 1 ]]
"$proof/bin/sentinel" role verify --root "$offline_project" --path "$report" \
  --expected-sha256 "$(shasum -a 256 "$report_file" | awk '{print $1}')" \
  --ci-result .ingen/artifacts/role-ci.json >"$proof/offline-verify.log"
jq -e '.status == "passed" and .report.status == "completed"' "$offline_project/.ingen/artifacts/role-ci.json" >/dev/null
report_digest="$(shasum -a 256 "$report_file" | awk '{print $1}')"
prompt_digest="$(shasum -a 256 "$offline_project/$snapshot_path" | awk '{print $1}')"
custody="$proof/offline-custody"
"$proof/bin/lockwood" import-role-execution --root "$custody" --project-root "$offline_project" \
  --path "$report" --expected-digest "$report_digest" --id native-context-offline >"$proof/offline-custody-import.log"
"$proof/bin/lockwood" verify --root "$custody" --id native-context-offline >"$proof/offline-custody-verify.log"
"$proof/bin/lockwood" get --root "$custody" --output "$proof/retrieved-prompt.txt" \
  "sha256:$prompt_digest" >"$proof/offline-custody-get.log"
cmp "$offline_project/$snapshot_path" "$proof/retrieved-prompt.txt"

printf 'Offline typed-agent fixture passed: exact prompt bytes, private fresh context, seeded prior-context denial, filtered credentials, disabled network with live loopback control target, and verified role evidence.\n'

if [[ "$live" == true ]]; then
  HERDR_ENV=1 "$proof/bin/sentinel" session native-snapshot --socket "$socket_path" >"$proof/live-snapshot-before.json"
  expect_failure() {
    local actual=0
    "$@" || actual=$?
    [[ "$actual" != 0 ]]
  }

  live_owned_journals=()
  run_live_case() {
    local mode="$1" index="$2"
    local prefix="$proof/live-$index"
    local journal stdout_rel state report_path execution_id live_project="$proof/live-$index"
    live_projects+=("$live_project")
    new_project "$live_project" "native-context-live-$index"
    cat >"$live_project/.ingen/contract/live-$index.txt" <<EOF
NATIVE_FIXTURE_MODE=$mode
FORBIDDEN_CONTEXT=$live_project/.ingen/legacy-context/private.json
LEGACY_HOME=$live_project/.ingen/legacy-context/home
LEGACY_CODEX_HOME=$live_project/.ingen/legacy-context/codex
NETWORK_PROBE_ADDR=$probe_address
This is a bounded offline executable hosted in a Herdr workspace; it does not call Codex.
EOF
    HERDR_ENV=1 "$proof/bin/sentinel" session spawn --provider herdr --socket "$socket_path" \
      --root "$live_project" --workspace .ingen/workspace.yaml --receipt .ingen/artifacts/run.json \
      --role implementation --isolate --agent codex --agent-executable "$proof/bin/codex-fixture" \
      --prompt-file ".ingen/contract/live-$index.txt" >"$prefix-spawn.log" 2>&1
    journal="$(awk '$1 == "journal:" {print $2}' "$prefix-spawn.log")"
    [[ -n "$journal" && -f "$live_project/$journal" ]]
    live_owned_journals+=("$journal")
    execution_id="$(jq -r '.intent.argv as $argv | [range(0; $argv|length) as $i | select($argv[$i] == "--execution-id") | $argv[$i+1]][0] // empty' "$live_project/$journal")"
    [[ -n "$execution_id" ]]
    stdout_rel=".ingen/artifacts/role-executions/$execution_id.stdout"
    local ready=false
    for _ in $(seq 1 300); do
      state="$(jq -r '.state' "$live_project/$journal")"
      if [[ "$state" == running && -f "$live_project/$stdout_rel" ]] && grep -q '"type":"fixture-started"' "$live_project/$stdout_rel"; then
        ready=true
        break
      fi
      case "$state" in completed|failed|canceled|indeterminate) break ;; esac
      sleep 0.1
    done
    [[ "$ready" == true ]] || { echo "owned fixture did not reach running state: $mode ($state)" >&2; return 1; }

    cp "$live_project/$journal" "$prefix-journal-before-recover.json"
    expect_failure env HERDR_ENV=1 "$proof/bin/sentinel" session native-recover \
      --socket "$socket_path" --root "$live_project" --path "$journal" >"$prefix-recover-active.log" 2>&1
    grep -Fq 'holds the execution lease' "$prefix-recover-active.log"
    cmp "$prefix-journal-before-recover.json" "$live_project/$journal"
    expect_failure env HERDR_ENV=1 "$proof/bin/sentinel" session native-recover \
      --socket "$socket_path" --root "$live_project" --path "$journal" >"$prefix-recover-active-again.log" 2>&1
    grep -Fq 'holds the execution lease' "$prefix-recover-active-again.log"
    cmp "$prefix-journal-before-recover.json" "$live_project/$journal"
    expect_failure env HERDR_ENV=1 "$proof/bin/sentinel" session execute-native \
      --root "$live_project" --path "$journal" >"$prefix-duplicate-execute.log" 2>&1
    grep -Eq 'already held|already claimed|duplicate execution is refused' "$prefix-duplicate-execute.log"
    HERDR_ENV=1 "$proof/bin/sentinel" session native-process-info --socket "$socket_path" \
      --root "$live_project" --path "$journal" >"$prefix-process-info.json"
    jq -e '.trust == "local-observation-unverified" and .process_info.pane_id == .host.pane_id and .process_info.foreground_process_group_id > 0 and ((.process_info.foreground_processes // []) | length > 0)' "$prefix-process-info.json" >/dev/null
    HERDR_ENV=1 "$proof/bin/sentinel" session native-cancel --socket "$socket_path" \
      --root "$live_project" --path "$journal" >"$prefix-cancel.log"
    state=""
    for _ in $(seq 1 300); do
      state="$(jq -r '.state' "$live_project/$journal")"
      case "$state" in canceled|failed|completed|indeterminate) break ;; esac
      sleep 0.1
    done
    [[ "$state" == canceled ]]
    if [[ "$mode" == graceful ]]; then
      "$proof/bin/sentinel" session native-collect --root "$live_project" --path "$journal" \
        --receipt .ingen/artifacts/run.json >"$prefix-collect.log"
      report_path="$(python3 - "$live_project" <<'PY'
import json, sys
from pathlib import Path
root = Path(sys.argv[1])
items = []
for path in (root / ".ingen/artifacts/role-executions").glob("*.json"):
    if path.name.endswith(".policy.json"):
        continue
    try:
        data = json.loads(path.read_bytes())
    except Exception:
        continue
    if data.get("schema") == "ingen.sentinel-role-execution/v1" and data.get("agent_context", {}).get("prompt_source_path", "").endswith("live-1.txt"):
        items.append(path.relative_to(root).as_posix())
if len(items) != 1:
    raise SystemExit(f"expected one graceful role report: {items}")
print(items[0])
PY
)"
      jq -e '.status == "canceled" and .exit_code == 0' "$live_project/$report_path" >/dev/null
      local verify_code=0
      "$proof/bin/sentinel" role verify --root "$live_project" --path "$report_path" \
        --expected-sha256 "$(shasum -a 256 "$live_project/$report_path" | awk '{print $1}')" \
        --ci-result .ingen/artifacts/live-role-ci.json >"$prefix-role-verify.log" || verify_code=$?
      [[ "$verify_code" == 2 ]]
      jq -e '.status == "error" and .exit_code == 2 and .report.status == "canceled" and .report.exit_code == 0' "$live_project/.ingen/artifacts/live-role-ci.json" >/dev/null
    else
      "$proof/bin/sentinel" session native-collect --root "$live_project" --path "$journal" \
        --receipt .ingen/artifacts/run.json >"$prefix-collect.log"
      report_path="$(python3 - "$live_project" <<'PY'
import json, sys
from pathlib import Path
root = Path(sys.argv[1])
items = []
for path in (root / ".ingen/artifacts/role-executions").glob("*.json"):
    if path.name.endswith(".policy.json"):
        continue
    try:
        data = json.loads(path.read_bytes())
    except Exception:
        continue
    if data.get("schema") == "ingen.sentinel-role-execution/v1" and data.get("agent_context", {}).get("prompt_source_path", "").endswith("live-2.txt"):
        items.append(path.relative_to(root).as_posix())
if len(items) > 1:
    raise SystemExit(f"duplicate ignored-signal role reports: {items}")
print(items[0] if items else "")
PY
)"
      if [[ -n "$report_path" ]]; then
        jq -e '.status == "canceled" and .exit_code == -1' "$live_project/$report_path" >/dev/null
        local verify_code=0
        "$proof/bin/sentinel" role verify --root "$live_project" --path "$report_path" \
          --expected-sha256 "$(shasum -a 256 "$live_project/$report_path" | awk '{print $1}')" \
          --ci-result .ingen/artifacts/ignore-role-ci.json >"$prefix-role-verify.log" || verify_code=$?
        [[ "$verify_code" == 2 ]]
        jq -e '.status == "error" and .exit_code == 2 and .report.status == "canceled" and .report.exit_code == -1' "$live_project/.ingen/artifacts/ignore-role-ci.json" >/dev/null
      else
        echo 'ignored-signal role wrapper did not publish its canceled report; collection should have failed closed' >&2
        return 1
      fi
    fi
    cp "$live_project/.ingen/artifacts/run.json" "$prefix-receipt-before-repeat.json"
    "$proof/bin/sentinel" session native-collect --root "$live_project" --path "$journal" \
      --receipt .ingen/artifacts/run.json >"$prefix-repeat-collect.log"
    cmp "$prefix-receipt-before-repeat.json" "$live_project/.ingen/artifacts/run.json"
    "$proof/bin/sentinel" session native-status --root "$live_project" --path "$journal" --format json >"$prefix-terminal-status.json"
    HERDR_ENV=1 "$proof/bin/sentinel" session native-close --socket "$socket_path" \
      --root "$live_project" --path "$journal" >"$prefix-close.json"
    live_closed_journals+=("$live_project:$journal")
    if [[ "$mode" == graceful ]]; then
      jq -e '.state == "canceled" and ([.events[] | select(.kind == "wrapper-terminal" and .exit_code == 1)] | length == 1)' "$live_project/$journal" >/dev/null
    else
      jq -e '.state == "canceled" and ([.events[] | select(.kind == "wrapper-terminal" and .exit_code == 1)] | length == 1)' "$live_project/$journal" >/dev/null
    fi
  }

  run_live_case graceful 1
  run_live_case ignore-interrupt 2
  HERDR_ENV=1 "$proof/bin/sentinel" session native-snapshot --socket "$socket_path" >"$proof/live-snapshot-after.json"
  jq -e --slurp '.[0].focused_workspace_id == .[1].focused_workspace_id and .[0].focused_tab_id == .[1].focused_tab_id and .[0].focused_pane_id == .[1].focused_pane_id and ([.[0].workspaces[].workspace_id] | sort) == ([.[1].workspaces[].workspace_id] | sort)' "$proof/live-snapshot-before.json" "$proof/live-snapshot-after.json" >/dev/null
  printf 'Live host proof passed for owned workspace create, active recovery immutability, duplicate execute refusal, exact-pane process observation, graceful and ignored-signal cancellation, repeated byte-identical collection, exact owned close, and before/after snapshot preservation.\n'
fi

python3 - "$proof" "$offline_project" "$report" "$report_digest" "$snapshot_path" "$prompt_digest" "$live" <<'PY'
import json, sys
from pathlib import Path
proof, project, report, report_sha, prompt_path, prompt_sha, live = sys.argv[1:]
value = {
    "schema": "ingen.acceptance-native-context-recovery/v1",
    "evidence_root": str(Path(proof).resolve()),
    "offline": {
        "status": "passed",
        "project_root": project,
        "agent": "offline-codex-compatible-fixture",
        "real_codex_invoked": False,
        "model_provider_contacted": False,
        "role_report": report,
        "role_report_sha256": report_sha,
        "prompt_snapshot": prompt_path,
        "prompt_snapshot_sha256": prompt_sha,
        "custody": "lockwood-verified-and-prompt-retrieved-byte-identically",
        "network_probe": "loopback control target was live; isolated fixture received explicit access denial",
        "native_session_tests": "see native-session-tests.log for lease, recovery, and cancel race tests",
    },
    "live_host": {
        "requested": live == "true",
        "result": ("passed" if live == "true" else "host operations are not run unless --live and an explicit socket are supplied"),
        "trust": "local host observations are unverified and not Herdr-authenticated",
    },
    "limitations": [
        "offline fixture is not Codex and does not attest to model behavior",
        "fixture uses no real model account or network service",
        "native host identity and process-info observations are local and unverified",
    ],
}
(Path(proof) / "acceptance-index.json").write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
PY
printf 'acceptance index: %s\n' "$proof/acceptance-index.json"
