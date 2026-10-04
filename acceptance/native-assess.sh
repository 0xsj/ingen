#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: acceptance/native-assess.sh --bin ABSOLUTE_SENTINEL

Builds a synthetic project using a fake Herdr client and the selected Sentinel
binary, then checks read-only native assessment against a completed wrapper,
drifted capture, and mismatched receipt. It makes no live host or provider
calls. Project and reports are retained under /private/tmp for review.
USAGE
}
fail() { printf 'native-assess acceptance: %s\n' "$1" >&2; exit 1; }

bin=
while (($#)); do
  case "$1" in
    --bin) (($# >= 2)) || fail "missing value for --bin"; bin=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$bin" = /* && -f "$bin" && ! -L "$bin" && -x "$bin" ]] || fail "--bin must name an absolute executable regular file"
bin=$(cd "$(dirname "$bin")" && pwd -P)/$(basename "$bin")
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"

case "$(uname -s)" in
  Darwin) default_tmp=/private/tmp ;;
  Linux) default_tmp=/tmp ;;
  *) fail "unsupported proof host platform" ;;
esac
proof_dir=$(mktemp -d "${INGEN_NATIVE_ASSESS_TMPDIR:-$default_tmp}/ingen-native-assess.XXXXXX")
proof_dir=$(cd "$proof_dir" && pwd -P)
project_root="$proof_dir/project"
mkdir -m 700 "$project_root"
binary_sha=$(shasum -a 256 "$bin" | awk '{print $1}')
[[ "$binary_sha" =~ ^[0-9a-f]{64}$ ]] || fail "could not fingerprint Sentinel binary"
"$bin" version --format json > "$proof_dir/sentinel-version.json"

INGEN_NATIVE_ASSESS_FIXTURE_ROOT="$project_root" INGEN_NATIVE_ASSESS_SENTINEL="$bin" \
  GOCACHE="${GOCACHE:-$(pwd -P)/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$(pwd -P)/.cache/go-mod}" \
  go test ./herdr-sentinel/internal/nativesession -run '^TestWriteNativeAssessmentFixtureForInstalledCLI$' -count=1

journal_rel=$(python3 - "$project_root" <<'PY'
import pathlib, sys
directory = pathlib.Path(sys.argv[1]) / ".ingen/artifacts/native-sessions"
paths = sorted(p for p in directory.glob("*.json") if not p.name.endswith(".lock"))
if len(paths) != 1:
    raise SystemExit(f"expected one synthetic native journal, found {len(paths)}")
print(".ingen/artifacts/native-sessions/" + paths[0].name)
PY
)

tree_snapshot() {
  python3 - "$project_root" <<'PY'
import hashlib, json, os, pathlib, stat, sys
root = pathlib.Path(sys.argv[1])
rows = []
for current, dirs, files in os.walk(root, followlinks=False):
    dirs.sort()
    files.sort()
    base = pathlib.Path(current)
    for name in dirs:
        path = base / name
        info = path.lstat()
        if not stat.S_ISDIR(info.st_mode):
            raise SystemExit("project tree contains symlink or special directory")
        rows.append({"path":path.relative_to(root).as_posix(),"type":"directory","mode":format(stat.S_IMODE(info.st_mode),"04o")})
    for name in files:
        path = base / name
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode):
            raise SystemExit("project tree contains symlink or special file")
        h = hashlib.sha256()
        with path.open("rb") as stream:
            for block in iter(lambda: stream.read(1024*1024), b""):
                h.update(block)
        rows.append({"path":path.relative_to(root).as_posix(),"type":"file","mode":format(stat.S_IMODE(info.st_mode),"04o"),"size":info.st_size,"sha256":h.hexdigest()})
print(json.dumps(rows,sort_keys=True,separators=(",",":")))
PY
}

tree_digest() { printf '%s' "$1" | shasum -a 256 | awk '{print $1}'; }

before_snapshot=$(tree_snapshot)
before=$(tree_digest "$before_snapshot")
printf '%s\n' "$before_snapshot" > "$proof_dir/project-before-assessments.json"
"$bin" session native-assess --root "$project_root" --path "$journal_rel" --format json > "$proof_dir/completed.json"
"$bin" session native-assess --root "$project_root" --path "$journal_rel" --format json > "$proof_dir/completed-repeat.json"
python3 - "$proof_dir/completed.json" <<'PY'
import json, pathlib, sys
d = json.loads(pathlib.Path(sys.argv[1]).read_text())
assert d["schema"] == "ingen.sentinel-native-recovery-assessment/v1"
assert d["status"] == "assessed" and d["state"] == "completed"
assert d["execution_lease"]["status"] == "free"
assert d["receipt"]["status"] == "matched"
assert d["terminal_evidence"]["status"] == "matched"
assert "does not contact Herdr" in d["limitations"][0]
PY
python3 - "$proof_dir/completed.json" "$proof_dir/completed-repeat.json" <<'PY'
import json, pathlib, sys
first, second = [json.loads(pathlib.Path(path).read_text()) for path in sys.argv[1:]]
assert first["schema"] == second["schema"] == "ingen.sentinel-native-recovery-assessment/v1"
first.pop("assessed_at", None)
second.pop("assessed_at", None)
assert first == second, "repeated assessments differ beyond assessed_at"
PY
after_snapshot=$(tree_snapshot)
after=$(tree_digest "$after_snapshot")
printf '%s\n' "$after_snapshot" > "$proof_dir/project-after-assessments.json"
[[ "$before" = "$after" ]] || fail "assessment changed the project tree"
INGEN_NATIVE_ASSESS_REPORT="$proof_dir/completed.json" GOCACHE="${GOCACHE:-$(pwd -P)/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$(pwd -P)/.cache/go-mod}" go test ./herdr-sentinel/spec -run '^TestInstalledNativeRecoveryAssessmentMatchesSchema$' -count=1

stdout_rel=$(python3 - "$proof_dir/completed.json" <<'PY'
import json, pathlib, sys
print(json.loads(pathlib.Path(sys.argv[1]).read_text())["terminal_evidence"]["stdout_path"])
PY
)
cp "$project_root/$stdout_rel" "$proof_dir/stdout.original"
printf 'tampered evidence\n' > "$project_root/$stdout_rel"
cp "$project_root/$stdout_rel" "$proof_dir/drift.stdout"
drift_input_snapshot=$(tree_snapshot)
drift_input_digest=$(tree_digest "$drift_input_snapshot")
printf '%s\n' "$drift_input_snapshot" > "$proof_dir/drift-input-project.json"
if "$bin" session native-assess --root "$project_root" --path "$journal_rel" --format json > "$proof_dir/drift.json"; then
  fail "drifted capture returned success"
else
  code=$?
  [[ "$code" = 1 ]] || fail "drifted capture returned exit $code, want 1"
fi
python3 - "$proof_dir/drift.json" <<'PY'
import json, pathlib, sys
d = json.loads(pathlib.Path(sys.argv[1]).read_text())
assert d["status"] == "uncertain" and d["terminal_evidence"]["status"] == "drift"
assert d["terminal_evidence"].get("observed_stdout_sha256")
assert d["recommended_action"] == "manual-review"
PY
after_snapshot=$(tree_snapshot)
after=$(tree_digest "$after_snapshot")
printf '%s\n' "$after_snapshot" > "$proof_dir/drift-after-assessment-project.json"
[[ "$drift_input_digest" = "$after" ]] || fail "drift assessment changed the altered-input project"
cp "$proof_dir/stdout.original" "$project_root/$stdout_rel"

receipt_rel=$(python3 - "$proof_dir/completed.json" <<'PY'
import json, pathlib, sys
print(json.loads(pathlib.Path(sys.argv[1]).read_text())["receipt"]["path"])
PY
)
cp "$project_root/$receipt_rel" "$proof_dir/receipt.original"
python3 - "$project_root/$receipt_rel" <<'PY'
import json, pathlib, sys
p = pathlib.Path(sys.argv[1])
d = json.loads(p.read_text())
d["run_id"] = "00000000-0000-4000-8000-000000000000"
p.write_text(json.dumps(d, indent=2) + "\n")
PY
cp "$project_root/$receipt_rel" "$proof_dir/receipt-mismatch.receipt.json"
receipt_input_snapshot=$(tree_snapshot)
receipt_input_digest=$(tree_digest "$receipt_input_snapshot")
printf '%s\n' "$receipt_input_snapshot" > "$proof_dir/receipt-mismatch-input-project.json"
if "$bin" session native-assess --root "$project_root" --path "$journal_rel" --format json > "$proof_dir/receipt-mismatch.json"; then
  fail "mismatched receipt returned success"
else
  code=$?
  [[ "$code" = 1 ]] || fail "mismatched receipt returned exit $code, want 1"
fi
python3 - "$proof_dir/receipt-mismatch.json" <<'PY'
import json, pathlib, sys
d = json.loads(pathlib.Path(sys.argv[1]).read_text())
assert d["status"] == "uncertain" and d["receipt"]["status"] == "mismatched"
assert d["recommended_action"] == "manual-review"
PY
after_snapshot=$(tree_snapshot)
after=$(tree_digest "$after_snapshot")
printf '%s\n' "$after_snapshot" > "$proof_dir/receipt-mismatch-after-assessment-project.json"
[[ "$receipt_input_digest" = "$after" ]] || fail "receipt assessment changed the mismatched-input project"
cp "$proof_dir/receipt.original" "$project_root/$receipt_rel"
restored_snapshot=$(tree_snapshot)
restored_digest=$(tree_digest "$restored_snapshot")
printf '%s\n' "$restored_snapshot" > "$proof_dir/project-restored.json"
[[ "$before" = "$restored_digest" ]] || fail "receipt restoration did not recover baseline project tree"

python3 - "$proof_dir" "$bin" "$binary_sha" "$journal_rel" "$project_root" "$before" "$drift_input_digest" "$receipt_input_digest" <<'PY'
import hashlib, json, pathlib, sys
directory, binary, digest, journal = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3], sys.argv[4]
project, baseline_tree_sha, drift_tree_sha, mismatch_tree_sha = sys.argv[5:]
files = {}
for name in ("completed.json", "completed-repeat.json", "drift.json", "receipt-mismatch.json",
             "stdout.original", "drift.stdout", "receipt.original", "receipt-mismatch.receipt.json",
             "sentinel-version.json",
             "project-before-assessments.json", "project-after-assessments.json",
             "project-restored.json", "drift-input-project.json",
             "drift-after-assessment-project.json", "receipt-mismatch-input-project.json",
             "receipt-mismatch-after-assessment-project.json"):
    data = (directory / name).read_bytes()
    files[name] = hashlib.sha256(data).hexdigest()
completed = json.loads((directory / "completed.json").read_text())
receipt_path = completed["receipt"]["path"]
receipt = json.loads((pathlib.Path(project) / receipt_path).read_text())
version = json.loads((directory / "sentinel-version.json").read_text())
files["journal"] = {"path":journal,"sha256":hashlib.sha256((pathlib.Path(project)/journal).read_bytes()).hexdigest()}
files["receipt"] = {"path":receipt_path,"sha256":hashlib.sha256((pathlib.Path(project)/receipt_path).read_bytes()).hexdigest()}
for kind, field in (("stdout","stdout_path"),("stderr","stderr_path")):
    rel = completed["terminal_evidence"].get(field)
    if rel:
        files[kind] = {"path":rel,"sha256":hashlib.sha256((pathlib.Path(project)/rel).read_bytes()).hexdigest()}
workspace_path = receipt.get("workspace",{}).get("file",{}).get("path")
project_evidence = {
    "journal": {"path":journal,"sha256":hashlib.sha256((pathlib.Path(project)/journal).read_bytes()).hexdigest()},
    "receipt": {"path":receipt_path,"sha256":hashlib.sha256((pathlib.Path(project)/receipt_path).read_bytes()).hexdigest()},
}
for kind, field in (("stdout","stdout_path"),("stderr","stderr_path")):
    rel = completed["terminal_evidence"].get(field)
    if rel:
        project_evidence[kind] = {"path":rel,"sha256":hashlib.sha256((pathlib.Path(project)/rel).read_bytes()).hexdigest()}
if workspace_path:
    project_evidence["workspace_manifest"] = {"path":workspace_path,"sha256":hashlib.sha256((pathlib.Path(project)/workspace_path).read_bytes()).hexdigest()}
if version.get("schema") != "ingen.tool-version/v1" or version.get("name") != "sentinel":
    raise SystemExit("Sentinel version report identity is invalid")
report_names = ("completed.json", "completed-repeat.json", "drift.json", "receipt-mismatch.json")
snapshot_names = ("project-before-assessments.json", "project-after-assessments.json", "project-restored.json",
                  "drift-input-project.json", "drift-after-assessment-project.json",
                  "receipt-mismatch-input-project.json", "receipt-mismatch-after-assessment-project.json")
index = {"schema":"ingen.native-assess-acceptance-index/v1", "binary":binary,
         "binary_sha256":digest, "synthetic_project":"project", "journal_path":journal,
         "sentinel_version":{"report":version,"sha256":files["sentinel-version.json"]},"baseline_tree_sha256":baseline_tree_sha,
         "drift_input_tree_sha256":drift_tree_sha,"receipt_mismatch_input_tree_sha256":mismatch_tree_sha,
         "report_sha256":{name:files[name] for name in report_names},
         "project_snapshot_sha256":{name:files[name] for name in snapshot_names},
         "project_evidence":project_evidence,
         "report_inputs":{
             "completed.json":baseline_tree_sha,
             "completed-repeat.json":baseline_tree_sha,
             "drift.json":drift_tree_sha,
             "receipt-mismatch.json":mismatch_tree_sha,
         },
         "evidence_files":files, "limitations":["fake Herdr client used only by Go fixture producer", "no live host, provider, relaunch, signal, or recovery action"]}
(directory / "acceptance-index.json").write_text(json.dumps(index, indent=2) + "\n")
PY
printf 'native-assess acceptance passed: %s\n' "$proof_dir"
printf 'binary-sha256: %s\n' "$binary_sha"
