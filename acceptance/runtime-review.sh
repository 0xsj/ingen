#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: acceptance/runtime-review.sh --bin ABS_SENTINEL

Combines the native assessment fixture with a local synthetic agent comparison.
The fixture uses a fake Herdr client and /usr/bin/true. The comparison selects
/usr/bin/false and should be unsupported with zero mock-broker requests.
USAGE
}
fail() { printf 'runtime-review: %s\n' "$1" >&2; exit 1; }

bin=
while (($#)); do
  case "$1" in
    --bin) (($# >= 2)) || fail "missing value for --bin"; bin=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$bin" = /* && -f "$bin" && ! -L "$bin" && -x "$bin" ]] || fail "--bin must be an absolute executable regular file"
bin=$(cd "$(dirname "$bin")" && pwd -P)/$(basename "$bin")
repo=$(cd "$(dirname "$0")/.." && pwd -P)
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"
command -v go >/dev/null 2>&1 || fail "Go is required for the opt-in synthetic fixture"

case "$(uname -s)" in
  Darwin) proof_parent=/private/tmp ;;
  Linux) proof_parent=/tmp ;;
  *) fail "unsupported proof host platform" ;;
esac
proof=$(mktemp -d "$proof_parent/ingen-runtime-review.XXXXXX")
proof=$(cd "$proof" && pwd -P)
trap 'printf "runtime review evidence: %s\n" "$proof"' EXIT
binary_sha=$(shasum -a 256 "$bin" | awk '{print $1}')
[[ "$binary_sha" =~ ^[0-9a-f]{64}$ ]] || fail "could not fingerprint Sentinel binary"
"$bin" version --format json > "$proof/sentinel-version.json" || fail "installed Sentinel version report failed"

INGEN_NATIVE_ASSESS_TMPDIR="$proof" bash "$repo/acceptance/native-assess.sh" --bin "$bin" > "$proof/native-assess.log" 2>&1 || fail "native assessment fixture failed; inspect native-assess.log"
native_proof=$(awk '/^native-assess acceptance passed: / {print $NF}' "$proof/native-assess.log" | tail -1)
[[ "$native_proof" = "$proof/"* && -f "$native_proof/acceptance-index.json" ]] || fail "native assessment proof index is unavailable under the owned proof root"

if "$bin" agent compare --agent-executable /usr/bin/false --timeout-seconds 5 > "$proof/agent-compare.json"; then
  fail "false executable was unexpectedly supported"
else
  code=$?
  [[ "$code" = 1 ]] || fail "agent compare returned exit $code, expected unsupported exit 1"
fi

python3 - "$proof" "$native_proof" "$bin" "$binary_sha" <<'PY'
import hashlib, json, os, pathlib, sys
proof, native_text, binary, expected_sha = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
native = pathlib.Path(native_text).resolve(strict=True)
native_index_path = native / "acceptance-index.json"
native_index = json.loads(native_index_path.read_text(encoding="utf-8"))
report_digests = native_index.get("report_sha256")
expected_reports = {"completed.json", "completed-repeat.json", "drift.json", "receipt-mismatch.json"}
if not isinstance(report_digests, dict) or set(report_digests) != expected_reports:
    raise SystemExit("native assessment proof does not contain all four report digests")
if any(not isinstance(digest, str) or len(digest) != 64 for digest in report_digests.values()):
    raise SystemExit("native assessment report digest catalog is invalid")
matrix_path = proof / "agent-compare.json"
matrix = json.loads(matrix_path.read_text(encoding="utf-8"))
version_path = proof / "sentinel-version.json"
version = json.loads(version_path.read_text(encoding="utf-8"))
if version.get("schema") != "ingen.tool-version/v1" or version.get("name") != "sentinel":
    raise SystemExit("Sentinel version report identity is invalid")
version_binding = native_index.get("sentinel_version") or {}
if native_index.get("binary_sha256") != expected_sha or version_binding.get("report") != version or version_binding.get("sha256") != hashlib.sha256(version_path.read_bytes()).hexdigest():
    raise SystemExit("native assessment proof does not bind this installed Sentinel")
if matrix.get("schema") != "ingen.sentinel-codex-compatibility-matrix/v1" or matrix.get("status") != "unsupported" or matrix.get("synthetic") is not True:
    raise SystemExit("agent comparison did not return the expected synthetic unsupported result")
if len(matrix.get("candidates", [])) != 1:
    raise SystemExit("agent comparison candidate count is invalid")
candidate = matrix["candidates"][0]
canonical_false = os.path.realpath("/usr/bin/false")
if candidate.get("status") != "unsupported" or candidate.get("executable_path") != canonical_false:
    raise SystemExit("agent comparison did not preserve the selected canonical false executable")
diagnostic = candidate.get("diagnostic") or {}
before_sha = candidate.get("executable_sha256_before")
if not before_sha or candidate.get("executable_sha256_after") != before_sha or diagnostic.get("executable_sha256") != before_sha:
    raise SystemExit("agent comparison executable identity changed during the probe")
broker = diagnostic.get("broker") or {}
if diagnostic.get("mock_round_trips") != 0 or broker.get("requests_received") != 0 or broker.get("requests_forwarded") != 0 or broker.get("in_flight") != 0:
    raise SystemExit("false candidate caused a synthetic broker request")
if diagnostic.get("process_started") is not True or diagnostic.get("exit_code") in (None, 0):
    raise SystemExit("false candidate was not actually started and rejected")

def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()

files = []
for path in sorted(p for p in proof.rglob("*") if p.is_file() and not p.is_symlink() and p.name != "acceptance-index.json"):
    files.append({"path": path.relative_to(proof).as_posix(), "size": path.stat().st_size, "sha256": sha(path)})
index = {
    "schema": "ingen.runtime-review-acceptance/v1",
    "status": "passed",
    "scope": "installed Sentinel runtime and read-only synthetic diagnostics",
    "sentinel": {
        "path": binary,
        "sha256": expected_sha,
        "version_report": {"path": "sentinel-version.json", "sha256": sha(version_path), "identity": version},
    },
    "native_assessment": {
        "acceptance_index": {
            "path": native_index_path.relative_to(proof).as_posix(),
            "sha256": sha(native_index_path),
        },
        "fixture_project": (native / "project").relative_to(proof).as_posix(),
        "report_sha256": report_digests,
        "baseline_tree_sha256": native_index.get("baseline_tree_sha256"),
        "tree_unchanged_after_repeated_assessment": True,
        "tampered_capture": "assessment returned uncertain with terminal evidence drift; its project snapshot remained unchanged",
        "receipt_mismatch": "assessment returned uncertain; receipt was restored after inspection",
    },
    "agent_compare": {
        "report": {"path": "agent-compare.json", "sha256": sha(matrix_path)},
        "candidate": candidate,
        "mock_requests": 0,
    },
    "files": files,
    "limitations": [
        "native journal and host binding come from the fake-host fixture producer; no live Herdr socket or workspace was used",
        "the fixture launches the real Sentinel wrapper with /usr/bin/true but does not establish governed or contained execution",
        "the agent comparison starts /usr/bin/false and creates a local synthetic broker listener; no provider or external service call is made",
        "this acceptance is local diagnostics only and is not a release approval or host attestation",
    ],
}
out = proof / "acceptance-index.json"
with out.open("x", encoding="utf-8") as stream:
    json.dump(index, stream, sort_keys=True, indent=2)
    stream.write("\n")
PY
printf 'runtime review passed: %s\n' "$proof"
printf 'acceptance index: %s/acceptance-index.json\n' "$proof"
