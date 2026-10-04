#!/usr/bin/env bash
# Exercise the production Sentinel Codex broker path without contacting a provider.
# The contained fixture sends only locally rejected requests and checks the
# broker port separately from an independently proven reachable loopback port.
set -euo pipefail

checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
proof="$(mktemp -d /private/tmp/ingen-codex-broker.XXXXXX)"
receiver_pid=""
cleanup() {
  if [[ -n "$receiver_pid" ]] && kill -0 "$receiver_pid" 2>/dev/null; then
    kill "$receiver_pid" 2>/dev/null || true
    wait "$receiver_pid" 2>/dev/null || true
  fi
  printf 'evidence root: %s\n' "$proof"
}
trap cleanup EXIT

if [[ "$(uname -s)" != Darwin ]]; then
  echo 'This acceptance check requires macOS Seatbelt.' >&2
  exit 2
fi
for tool in go python3 jq shasum; do
  command -v "$tool" >/dev/null || { echo "required tool is missing: $tool" >&2; exit 2; }
done
export GOCACHE="${GOCACHE:-$checkout/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$checkout/.cache/go-mod}"
mkdir -p "$proof/bin"
cd "$checkout"
go build -o "$proof/bin/sentinel" ./herdr-sentinel/cmd/sentinel
go build -o "$proof/bin/lockwood" ./lockwood/cmd/lockwood

cat >"$proof/codex-fixture.go" <<'GO'
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

type result struct {
	FixedProfile       bool  `json:"fixed_profile"`
	CredentialFree     bool  `json:"credential_free"`
	TokenPresent       bool  `json:"scoped_token_present"`
	BrokerResponses    []int `json:"broker_rejection_statuses"`
	OtherPortDenied    bool  `json:"other_reachable_loopback_port_denied"`
	TokenSHA256        string `json:"scoped_token_sha256"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "broker fixture failed:", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	model, endpoint := "", ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-m" && i+1 < len(args) {
			model = args[i+1]
		}
		if strings.HasPrefix(args[i], "model_providers.ingen_broker.base_url=") {
			endpoint = strings.TrimPrefix(args[i], "model_providers.ingen_broker.base_url=")
		}
	}
	if model != "gpt-5-codex" || endpoint == "" {
		return errors.New("fixed Codex broker profile arguments did not match")
	}
	expected := []string{
		"--no-daemon", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"--skip-git-repo-check", "--json", "--color", "never",
		"-c", "memories.use_memories=false", "-c", "memories.generate_memories=false",
		"-c", "project_doc_max_bytes=0", "-m", model,
		"-c", "model_provider=ingen_broker", "-c", "model_providers.ingen_broker.name=InGenBroker",
		"-c", "model_providers.ingen_broker.base_url=" + endpoint,
		"-c", "model_providers.ingen_broker.env_key=INGEN_CODEX_BROKER_TOKEN",
		"-c", "model_providers.ingen_broker.wire_api=responses",
		"-c", "model_providers.ingen_broker.requires_openai_auth=false",
		"-c", "model_providers.ingen_broker.supports_websockets=false",
		"-c", "model_providers.ingen_broker.request_max_retries=0",
		"-c", "model_providers.ingen_broker.stream_max_retries=0",
		"-c", "web_search=disabled", "-c", "features.multi_agent=false",
		"-c", "features.hooks=false", "-c", "features.remote_plugin=false",
		"-c", "features.shell_snapshot=false", "-c", "approval_policy=never",
		"-c", "sandbox_mode=danger-full-access", "-",
	}
	if len(args) != len(expected) {
		return errors.New("fixed Codex broker profile arguments did not match")
	}
	for index := range expected {
		if args[index] != expected[index] {
			return fmt.Errorf("fixed Codex argument %d did not match the typed profile", index)
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "/v1" || parsed.RawQuery != "" {
		return errors.New("local broker endpoint was absent or malformed")
	}
	token := os.Getenv("INGEN_CODEX_BROKER_TOKEN")
	if token == "" {
		return errors.New("scoped broker token was not provided")
	}
	if _, exists := os.LookupEnv("OPENAI_API_KEY"); exists {
		return errors.New("provider API key reached Codex child")
	}
	if _, exists := os.LookupEnv("OPENAI_BASE_URL"); exists {
		return errors.New("provider routing override reached Codex child")
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var probe string
	for _, line := range strings.Split(string(prompt), "\n") {
		if strings.HasPrefix(line, "CONTROL_PROBE=") {
			probe = strings.TrimPrefix(line, "CONTROL_PROBE=")
		}
	}
	if probe == "" {
		return errors.New("prompt lacked the prevalidated loopback control endpoint")
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	checks := []struct {
		path, authorization, body string
		want                     int
	}{
		{"/v1/responses", "Bearer " + token, `{"model":"wrong-model","input":"not forwarded"}`, http.StatusBadRequest},
		{"/v1/responses", "Bearer deliberately-wrong-token", `{"model":"gpt-5-codex","input":"not forwarded"}`, http.StatusUnauthorized},
		{"/v1/responses", "Bearer " + token, `{"model":"gpt-5-codex","input":"not forwarded","tools":[{"type":"web_search","name":"web"}]}`, http.StatusBadRequest},
		{"/v1/not-responses", "Bearer " + token, `{"model":"gpt-5-codex","input":"not forwarded"}`, http.StatusNotFound},
	}
	statuses := make([]int, 0, len(checks))
	for _, check := range checks {
		request, err := http.NewRequest(http.MethodPost, endpoint+strings.TrimPrefix(check.path, "/v1"), strings.NewReader(check.body))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", check.authorization)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("local broker request failed: %w", err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		statuses = append(statuses, response.StatusCode)
		if response.StatusCode != check.want {
			return fmt.Errorf("broker returned %d for a locally invalid request; expected %d", response.StatusCode, check.want)
		}
	}
	connection, err := net.DialTimeout("tcp", probe, 2*time.Second)
	otherPortDenied := false
	if err != nil {
		otherPortDenied = errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
	} else {
		_ = connection.Close()
		return errors.New("contained Codex unexpectedly connected to the separate reachable loopback port")
	}
	if !otherPortDenied {
		return fmt.Errorf("alternate loopback port was not explicitly denied by policy: %v", err)
	}
	tokenDigest := sha256.Sum256([]byte(token))
	output := result{FixedProfile: true, CredentialFree: true, TokenPresent: true, BrokerResponses: statuses, OtherPortDenied: otherPortDenied, TokenSHA256: hex.EncodeToString(tokenDigest[:])}
	return json.NewEncoder(os.Stdout).Encode(output)
}

GO
go build -o "$proof/bin/codex-fixture" "$proof/codex-fixture.go"

cat >"$proof/loopback-receiver.py" <<'PY'
import pathlib
import socket
import sys

address_file, accepted_file = map(pathlib.Path, sys.argv[1:])
listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
listener.bind(("127.0.0.1", 0))
listener.listen(8)
listener.settimeout(30)
address_file.write_text(f"127.0.0.1:{listener.getsockname()[1]}\n")
for index in range(2):
    try:
        connection, _ = listener.accept()
    except TimeoutError:
        break
    with connection:
        connection.settimeout(2)
        try:
            data = connection.recv(256)
        except OSError:
            data = b""
        if index == 0:
            accepted_file.write_bytes(data)
        else:
            (accepted_file.parent / "unexpected-contained-connect").write_bytes(data)
listener.close()
PY

receiver_address="$proof/receiver.address"
receiver_accepted="$proof/receiver.accepted"
python3 "$proof/loopback-receiver.py" "$receiver_address" "$receiver_accepted" >"$proof/receiver.log" 2>&1 &
receiver_pid=$!
for _ in $(seq 1 100); do
  [[ -s "$receiver_address" ]] && break
  sleep 0.05
done
[[ -s "$receiver_address" ]] || { cat "$proof/receiver.log" >&2; exit 1; }
control_probe="$(cat "$receiver_address")"
python3 - "$control_probe" <<'PY'
import socket, sys
host, port = sys.argv[1].rsplit(":", 1)
with socket.create_connection((host, int(port)), timeout=2) as connection:
    connection.sendall(b"host-preflight")
PY
for _ in $(seq 1 100); do
  [[ -s "$receiver_accepted" ]] && break
  sleep 0.05
done
[[ "$(cat "$receiver_accepted")" == host-preflight ]] || { echo 'alternate loopback control listener did not pass its host preflight' >&2; exit 1; }

project="$proof/project"
mkdir -p "$project"
"$proof/bin/sentinel" project init --root "$project" --id codex-broker-acceptance >"$proof/project-init.log"
python3 - "$project/.ingen/workspace.yaml" <<'PY'
from pathlib import Path
import re, sys
path = Path(sys.argv[1])
data = path.read_text()
roles = list(re.finditer(r"(?m)^( +)- id: implementation\n", data))
if len(roles) != 1:
    raise SystemExit("expected exactly one implementation role")
role = roles[0]
following = re.search(r"(?m)^" + re.escape(role.group(1)) + r"- id:", data[role.end():])
end = role.end() + following.start() if following else len(data)
block = data[role.start():end]
block, count = re.subn(r"(?m)^( +)- \.ingen/contract/contract\.json$", r"\1- .ingen/contract", block)
if count != 1:
    raise SystemExit("expected one implementation contract read grant")
path.write_text(data[:role.start()] + block + data[end:])
PY
"$proof/bin/sentinel" workspace validate "$project/.ingen/workspace.yaml" >"$proof/workspace-validate.log"
"$proof/bin/sentinel" run bootstrap --root "$project" --workspace .ingen/workspace.yaml \
  --output "$project/.ingen/artifacts/run.json" >"$proof/bootstrap.log"
prompt="$project/.ingen/contract/codex-broker-prompt.txt"
printf 'Broker acceptance fixture only. Do not contact any remote service.\nCONTROL_PROBE=%s\n' "$control_probe" >"$prompt"
fake_key='codex-broker-acceptance-invalid-credential'
env OPENAI_API_KEY="$fake_key" "$proof/bin/sentinel" session spawn \
  --root "$project" --workspace .ingen/workspace.yaml --receipt .ingen/artifacts/run.json \
  --role implementation --isolate --agent codex --agent-executable "$proof/bin/codex-fixture" \
  --prompt-file .ingen/contract/codex-broker-prompt.txt --agent-model gpt-5-codex \
  --agent-provider openai-broker --agent-credential-env OPENAI_API_KEY \
  --agent-max-requests 4 --agent-max-output-tokens 64 --agent-timeout-seconds 30 >"$proof/spawn.log" 2>&1

report="$(python3 - "$project" <<'PY'
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
report_file="$project/$report"
stdout_rel="$(jq -r '.stdout_path' "$report_file")"
policy_rel="$(jq -r '.policy_path' "$report_file")"
[[ -s "$project/$stdout_rel" && -s "$project/$policy_rel" ]]
endpoint="$(jq -r '.agent_context.broker.endpoint' "$report_file")"
case "$endpoint" in
  http://127.0.0.1:[0-9]*/v1) ;;
  *) echo "unexpected broker endpoint: $endpoint" >&2; exit 1 ;;
esac
broker_port="${endpoint#http://127.0.0.1:}"
broker_port="${broker_port%/v1}"
[[ "$broker_port" != "${control_probe##*:}" ]] || { echo 'broker and control ports unexpectedly match' >&2; exit 1; }
jq -e --arg endpoint "http://127.0.0.1:$broker_port/v1" \
  '.status == "completed" and .enforcement == "host-enforced" and .network_mode == "allowlist" and
   .agent_context.network_mode == "allowlist" and .agent_context.broker.endpoint == $endpoint and
   .broker_execution.status == "closed" and .broker_execution.stats.requests_received == 4 and
   .broker_execution.stats.requests_rejected == 4 and .broker_execution.stats.requests_forwarded == 0 and
   .broker_execution.stats.upstream_failures == 0 and .broker_execution.stats.in_flight == 0 and
   (.broker_execution.stats.closed_at != null) and (.limitations | any(. == "macOS Seatbelt localhost network permission includes local host addresses at the pinned TCP port; it is not an IPv4-only grant"))' \
  "$report_file" >/dev/null
policy_port="$broker_port"
jq -e --argjson port "$policy_port" \
  '.policy.network.mode == "allowlist" and (.policy.network.allow | length) == 1 and
   .policy.network.allow[0].host == "localhost" and .policy.network.allow[0].ports == [$port] and
   .policy.network.allow[0].direction == "outbound"' "$project/$policy_rel" >/dev/null
jq -e '.fixed_profile and .credential_free and .scoped_token_present and
       .broker_rejection_statuses == [400,401,400,404] and
       .other_reachable_loopback_port_denied and
       (.scoped_token_sha256 | test("^[0-9a-f]{64}$"))' \
  "$project/$stdout_rel" >/dev/null
[[ ! -e "$proof/unexpected-contained-connect" ]]

digest="$(shasum -a 256 "$report_file" | awk '{print $1}')"
"$proof/bin/sentinel" role verify --root "$project" --path "$report" \
  --expected-sha256 "$digest" --ci-result .ingen/artifacts/role-ci.json >"$proof/role-verify.log"
jq -e '.tool == "sentinel" and .kind == "role-execution" and .status == "passed" and .report.status == "completed" and .report.broker_execution.stats.requests_forwarded == 0' \
  "$project/.ingen/artifacts/role-ci.json" >/dev/null

custody="$project/.ingen/artifacts/custody"
"$proof/bin/lockwood" import-role-execution --root "$custody" --project-root "$project" \
  --path "$report" --expected-digest "$digest" --id codex-broker-acceptance >"$proof/custody-import.log"
"$proof/bin/lockwood" verify --root "$custody" --id codex-broker-acceptance >"$proof/custody-verify.log"
"$proof/bin/lockwood" get --root "$custody" --output "$proof/retrieved-report.json" "sha256:$digest" >"$proof/get-report.log"
cmp "$report_file" "$proof/retrieved-report.json"
prompt_snapshot="$(jq -r '.agent_context.prompt_snapshot_path' "$report_file")"
prompt_digest="$(jq -r '.agent_context.prompt_snapshot_sha256' "$report_file")"
"$proof/bin/lockwood" get --root "$custody" --output "$proof/retrieved-prompt.txt" "sha256:$prompt_digest" >"$proof/get-prompt.log"
cmp "$project/$prompt_snapshot" "$proof/retrieved-prompt.txt"

# The fixture reveals only a one-way token digest. Scan the entire persisted
# project for that digest's corresponding random token, and for the fake API key.
python3 - "$project" "$fake_key" "$project/$stdout_rel" <<'PY'
import hashlib, json, re, sys
from pathlib import Path
root, fake_key, stdout_path = Path(sys.argv[1]), sys.argv[2].encode(), Path(sys.argv[3])
fixture = json.loads(stdout_path.read_bytes())
target = bytes.fromhex(fixture["scoped_token_sha256"])
for path in root.rglob("*"):
    if not path.is_file():
        continue
    data = path.read_bytes()
    if fake_key in data:
        raise SystemExit(f"fake provider key persisted in {path}")
    for candidate in re.findall(rb"[A-Za-z0-9_-]{43}", data):
        if hashlib.sha256(candidate).digest() == target:
            raise SystemExit(f"scoped bearer token persisted in {path}")
PY

python3 - "$proof/acceptance-index.json" "$proof" "$project" "$report" "$digest" "$stdout_rel" "$policy_rel" "$prompt_snapshot" "$prompt_digest" "$broker_port" "$control_probe" "$proof/bin/sentinel" "$proof/bin/lockwood" "$proof/bin/codex-fixture" <<'PY'
import hashlib, json, sys
from pathlib import Path

index_path, proof, project = Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3])
report_rel, report_sha, stdout_rel, policy_rel, prompt_rel, prompt_sha, broker_port = sys.argv[4:11]
control_probe = sys.argv[11]
sentinel_bin, lockwood_bin, codex_bin = map(Path, sys.argv[12:15])
report_path = project / report_rel
report = json.loads(report_path.read_bytes())
stdout_path, policy_path, prompt_path = project / stdout_rel, project / policy_rel, project / prompt_rel
ci_path = project / ".ingen/artifacts/role-ci.json"

def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def file_ref(path, relative):
    return {"path": relative, "sha256": sha256(path)}

if sha256(report_path) != report_sha or sha256(prompt_path) != prompt_sha:
    raise SystemExit("acceptance index received a stale report or prompt digest")

document = {
    "schema": "ingen.acceptance-codex-broker/v1",
    "evidence_root": str(proof),
    "subject_root": str(project),
    "result": "passed",
    "profile": {
        "agent": "codex-cli-compatible-fixture",
        "model": report["agent_context"]["model"],
        "credential_source": "environment",
        "credential_env": report["agent_context"]["broker"]["credential_env"],
        "max_requests": report["agent_context"]["broker"]["max_requests"],
        "max_output_tokens": report["agent_context"]["broker"]["max_output_tokens"],
        "timeout_seconds": report["agent_context"]["broker"]["timeout_seconds"],
        "selected_endpoint": report["agent_context"]["broker"]["endpoint"],
        "network_allow": {"host": "localhost", "ports": [int(broker_port)], "direction": "outbound"},
        "control_loopback_port": {"address": control_probe, "result": "denied-with-EPERM-or-EACCES-after-host-preflight"},
    },
    "broker": {
        "requests_received": report["broker_execution"]["stats"]["requests_received"],
        "requests_rejected": report["broker_execution"]["stats"]["requests_rejected"],
        "requests_forwarded": report["broker_execution"]["stats"]["requests_forwarded"],
        "upstream_failures": report["broker_execution"]["stats"]["upstream_failures"],
        "in_flight_after_close": report["broker_execution"]["stats"]["in_flight"],
        "provider_calls": 0,
        "all_requests_rejected_before_forwarding": True,
    },
    "negative_probes": [
        {"kind": "model-mismatch", "status": 400},
        {"kind": "invalid-bearer", "status": 401},
        {"kind": "hosted-tool", "status": 400},
        {"kind": "wrong-route", "status": 404},
    ],
    "secret_checks": {
        "fake_api_key_value_persisted": False,
        "scoped_bearer_value_persisted": False,
        "scoped_bearer_test": "sha256-of-child-token compared against 43-character token candidates in project files",
        "index_contains_key_or_token_value": False,
    },
    "evidence": {
        "role_report": file_ref(report_path, report_rel),
        "role_stdout": file_ref(stdout_path, stdout_rel),
        "role_policy": file_ref(policy_path, policy_rel),
        "role_ci": file_ref(ci_path, ".ingen/artifacts/role-ci.json"),
        "prompt_snapshot": file_ref(prompt_path, prompt_rel),
        "lockwood_record_id": "codex-broker-acceptance",
        "lockwood_verification": "passed",
        "lockwood_report_retrieval": "byte-identical",
        "lockwood_prompt_retrieval": "byte-identical",
    },
    "binaries": {
        "sentinel_sha256": sha256(sentinel_bin),
        "lockwood_sha256": sha256(lockwood_bin),
        "codex_fixture_sha256": sha256(codex_bin),
    },
    "limitations": [
        "No provider request was forwarded; this checks local broker protocol and containment only.",
        "The Codex-compatible fixture is not the installed Codex CLI or a real model run.",
        "Provider inference, provider-side retention, and model behavior were not evaluated.",
        "Seatbelt localhost permission applies to local host addresses at the pinned port; it is not IPv4-only.",
    ],
}
index_path.write_text(json.dumps(document, sort_keys=True, indent=2) + "\n")
PY
if rg -n --fixed-strings "$fake_key" "$proof/acceptance-index.json" >/dev/null; then
  echo 'acceptance index contains the fake provider key' >&2
  exit 1
fi

printf 'Codex broker acceptance passed: exact localhost port policy, four pre-forward rejections, zero upstream attempts, alternate reachable loopback denied, secret-free role evidence, byte-identical Lockwood report/prompt retrieval, and acceptance-index.json.\n'
