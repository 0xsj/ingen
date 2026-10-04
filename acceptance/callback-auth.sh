#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: acceptance/callback-auth.sh --bin ABSOLUTE_SENTINEL [--project-root ABSENT_ABSOLUTE_PATH]

Exercises installed Sentinel file signing and authenticated callback ingestion
against a synthetic project. It makes no network or Herdr host calls. Public
receipts, event files, signed envelopes, and an acceptance index remain under
the project root. A random private key is created outside the project and
removed on exit.
USAGE
}
fail() { printf 'callback-auth acceptance: %s\n' "$1" >&2; exit 1; }

bin=
requested_root=
while (($#)); do
  case "$1" in
    --bin) (($# >= 2)) || fail "missing value for --bin"; bin=$2; shift 2 ;;
    --project-root) (($# >= 2)) || fail "missing value for --project-root"; requested_root=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ "$bin" = /* && -f "$bin" && ! -L "$bin" && -x "$bin" ]] || fail "--bin must name an absolute executable regular file"
bin=$(cd "$(dirname "$bin")" && pwd -P)/$(basename "$bin")
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"

if [[ -z "$requested_root" ]]; then
  private_dir=$(mktemp -d "${INGEN_CALLBACK_AUTH_TMPDIR:-/private/tmp}/ingen-callback-auth.XXXXXX")
  private_dir=$(cd "$private_dir" && pwd -P)
  project_root="$private_dir/project"
  key_dir="$private_dir/private"
  mkdir -m 700 "$key_dir"
else
  [[ "$requested_root" = /* && ! -e "$requested_root" && ! -L "$requested_root" ]] || fail "--project-root must be an absent absolute path"
  parent=$(cd "$(dirname "$requested_root")" && pwd -P) || fail "project root parent must exist"
  project_root="$parent/$(basename "$requested_root")"
  key_dir=$(mktemp -d "$parent/.sentinel-callback-key.XXXXXX")
  key_dir=$(cd "$key_dir" && pwd -P)
fi
mkdir -m 700 "$project_root"
project_root=$(cd "$project_root" && pwd -P)
[[ "$key_dir" != "$project_root" && "$key_dir" != "$project_root/"* ]] || fail "test key must be outside the project root"
key_path="$key_dir/operator.key"
cleanup() {
  [[ -n "$key_path" && -f "$key_path" && ! -L "$key_path" ]] && rm -f -- "$key_path"
  if [[ -n "$key_dir" && -d "$key_dir" && ! -L "$key_dir" ]]; then
    rm -f -- "$key_dir/version.json"
    for label in wrong-sender altered-payload altered-hmac conflicting-replay expired-envelope; do
      rm -f -- "$key_dir/$label.stdout" "$key_dir/$label.stderr"
    done
  fi
  [[ -n "$key_dir" && -d "$key_dir" && ! -L "$key_dir" ]] && rmdir -- "$key_dir" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
umask 077
dd if=/dev/urandom of="$key_path" bs=32 count=1 2>/dev/null
chmod 600 "$key_path"
[[ $(wc -c < "$key_path" | tr -d ' ') = 32 ]] || fail "failed to create the 32-byte test key"
binary_sha=$(shasum -a 256 "$bin" | awk '{print $1}')
[[ "$binary_sha" =~ ^[0-9a-f]{64}$ ]] || fail "could not fingerprint selected Sentinel binary"
"$bin" version --format json > "$key_dir/version.json"
python3 - "$key_dir/version.json" <<'PY'
import json, pathlib, sys
report = json.loads(pathlib.Path(sys.argv[1]).read_text())
if report.get("schema") != "ingen.tool-version/v1" or report.get("name") != "sentinel":
    raise SystemExit("selected binary returned an unexpected version report")
for field in ("version", "revision", "goos", "goarch", "toolchain"):
    if not isinstance(report.get(field), str) or not report[field]:
        raise SystemExit("selected binary version report is incomplete")
PY
version_report_sha=$(shasum -a 256 "$key_dir/version.json" | awk '{print $1}')

"$bin" project init --root "$project_root" --id callback-auth-acceptance > /dev/null
(cd "$project_root" && "$bin" run bootstrap --workspace .ingen/workspace.yaml --root "$project_root" --output .ingen/artifacts/sentinel-run.json > /dev/null)
artifact_dir="$project_root/.ingen/artifacts/callback-auth-acceptance"
mkdir -m 700 "$artifact_dir"

python3 - "$project_root" "$artifact_dir" <<'PY'
import datetime, json, pathlib, sys
root, out = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
receipt = json.loads((root / ".ingen/artifacts/sentinel-run.json").read_text())
at = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(seconds=10)).isoformat().replace("+00:00", "Z")
def event(event_id, outcome):
    return {"schema":"ingen.herdr-event/v1", "event_id":event_id, "run_id":receipt["run_id"],
            "workspace_id":receipt["workspace"]["id"], "workspace_version":receipt["workspace"]["version"],
            "type":"role-launched", "at":at, "role":"implementation", "workspace":".sentinel/implementation",
            "session_id":"callback-auth-acceptance", "receipt_status":"running", "outcome":outcome}
for name, value in (("original", event("acceptance-event-1", "started")),
                    ("conflict", event("acceptance-event-1", "different-start")),
                    ("expired", event("acceptance-event-expired", "started"))):
    (out / (name + ".jsonl")).write_text(json.dumps(value, separators=(",", ":")) + "\n")
PY

receipt_rel=.ingen/artifacts/sentinel-run.json
events_dir=.ingen/artifacts/callback-auth-acceptance
signed_rel=$events_dir/original.signed.json
conflict_signed_rel=$events_dir/conflict.signed.json
expired_signed_rel=$events_dir/expired.signed.json
bad_payload_rel=$events_dir/altered-payload.signed.json
bad_hmac_rel=$events_dir/altered-hmac.signed.json
index_rel=$events_dir/acceptance-index.json

sign_batch() {
  local events=$1 output=$2 ttl=${3:-300}
  (cd "$project_root" && "$bin" adapter herdr-sign-events --root "$project_root" --receipt "$receipt_rel" --events "$events_dir/$events" --key "$key_path" --sender local-acceptance-signer --output "$output" --lifetime-seconds "$ttl" > /dev/null)
}
sign_batch original.jsonl "$signed_rel"
sign_batch conflict.jsonl "$conflict_signed_rel"
sign_batch expired.jsonl "$expired_signed_rel" 1

schema_path=$(cd "$(dirname "$0")/.." && pwd -P)/herdr-sentinel/spec/herdr-signed-event-batch-v1.schema.json
python3 - "$project_root" "$signed_rel" "$bad_payload_rel" "$bad_hmac_rel" "$schema_path" <<'PY'
import base64, hashlib, json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
signed = root / sys.argv[2]
payload_out, hmac_out, schema_path = root / sys.argv[3], root / sys.argv[4], pathlib.Path(sys.argv[5])
envelope = json.loads(signed.read_text())
schema = json.loads(schema_path.read_text())
required, properties = set(schema["required"]), schema["properties"]
if set(envelope) != required or set(properties) != required:
    raise SystemExit("signed envelope fields differ from the published closed schema")
if envelope["schema"] != properties["schema"]["const"]:
    raise SystemExit("signed envelope schema identifier is wrong")
for field in ("key_id", "payload_sha256", "hmac_sha256"):
    if not re.fullmatch(properties[field]["pattern"], envelope[field]):
        raise SystemExit("signed envelope digest violates the published schema")
payload = base64.b64decode(envelope["payload_base64"], validate=True)
if hashlib.sha256(payload).hexdigest() != envelope["payload_sha256"]:
    raise SystemExit("signed payload digest is inconsistent")
altered = payload.replace(b'"outcome":"started"', b'"outcome":"tampered"', 1)
if altered == payload:
    raise SystemExit("could not locate the test event in signed payload")
bad_payload = dict(envelope)
bad_payload["payload_base64"] = base64.b64encode(altered).decode("ascii")
payload_out.write_text(json.dumps(bad_payload, separators=(",", ":")) + "\n")
bad_hmac = dict(envelope)
bad_hmac["hmac_sha256"] = "0" * 64
hmac_out.write_text(json.dumps(bad_hmac, separators=(",", ":")) + "\n")
PY

check_rejected_unchanged() {
  local label=$1 envelope=$2 sender=$3 before after
  before=$(shasum -a 256 "$project_root/$receipt_rel" | awk '{print $1}')
  if (cd "$project_root" && "$bin" adapter herdr-auth-events --root "$project_root" --receipt "$receipt_rel" --envelope "$envelope" --key "$key_path" --expected-sender "$sender" > "$key_dir/$label.stdout" 2> "$key_dir/$label.stderr"); then
    fail "$label unexpectedly succeeded"
  fi
  [[ ! -s "$key_dir/$label.stdout" ]] || fail "$label emitted unexpected stdout"
  after=$(shasum -a 256 "$project_root/$receipt_rel" | awk '{print $1}')
  [[ "$before" = "$after" ]] || fail "$label changed receipt bytes"
}

before=$(shasum -a 256 "$project_root/$receipt_rel" | awk '{print $1}')
(cd "$project_root" && "$bin" adapter herdr-auth-events --root "$project_root" --receipt "$receipt_rel" --envelope "$signed_rel" --key "$key_path" --expected-sender local-acceptance-signer > /dev/null)
after_first=$(shasum -a 256 "$project_root/$receipt_rel" | awk '{print $1}')
[[ "$before" != "$after_first" ]] || fail "valid signed batch did not update receipt"
(cd "$project_root" && "$bin" adapter herdr-auth-events --root "$project_root" --receipt "$receipt_rel" --envelope "$signed_rel" --key "$key_path" --expected-sender local-acceptance-signer > /dev/null)
after_replay=$(shasum -a 256 "$project_root/$receipt_rel" | awk '{print $1}')
[[ "$after_first" = "$after_replay" ]] || fail "exact replay rewrote receipt bytes"

check_rejected_unchanged wrong-sender "$signed_rel" wrong-sender
check_rejected_unchanged altered-payload "$bad_payload_rel" local-acceptance-signer
check_rejected_unchanged altered-hmac "$bad_hmac_rel" local-acceptance-signer
check_rejected_unchanged conflicting-replay "$conflict_signed_rel" local-acceptance-signer
sleep 2
check_rejected_unchanged expired-envelope "$expired_signed_rel" local-acceptance-signer

python3 - "$project_root" "$index_rel" "$after_replay" "$bin" "$binary_sha" "$version_report_sha" "$key_dir/version.json" <<'PY'
import datetime, hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
index_path = root / sys.argv[2]
expected_receipt_sha = sys.argv[3]
binary_path, binary_sha, version_report_sha = pathlib.Path(sys.argv[4]), sys.argv[5], sys.argv[6]
version_path = pathlib.Path(sys.argv[7])
if hashlib.sha256(binary_path.read_bytes()).hexdigest() != binary_sha:
    raise SystemExit("selected Sentinel binary changed during acceptance")
version_bytes = version_path.read_bytes()
if hashlib.sha256(version_bytes).hexdigest() != version_report_sha:
    raise SystemExit("selected Sentinel version report changed during acceptance")
version = json.loads(version_bytes)
items = [
    ("sentinel-run-receipt", ".ingen/artifacts/sentinel-run.json"),
    ("original-jsonl", ".ingen/artifacts/callback-auth-acceptance/original.jsonl"),
    ("conflicting-jsonl", ".ingen/artifacts/callback-auth-acceptance/conflict.jsonl"),
    ("expired-jsonl", ".ingen/artifacts/callback-auth-acceptance/expired.jsonl"),
    ("signed-envelope", ".ingen/artifacts/callback-auth-acceptance/original.signed.json"),
    ("conflicting-envelope", ".ingen/artifacts/callback-auth-acceptance/conflict.signed.json"),
    ("expired-envelope", ".ingen/artifacts/callback-auth-acceptance/expired.signed.json"),
    ("altered-payload-envelope", ".ingen/artifacts/callback-auth-acceptance/altered-payload.signed.json"),
    ("altered-hmac-envelope", ".ingen/artifacts/callback-auth-acceptance/altered-hmac.signed.json"),
]
files = [{"kind":kind, "path":path, "sha256":hashlib.sha256((root/path).read_bytes()).hexdigest()} for kind,path in items]
receipt_sha = next(x["sha256"] for x in files if x["kind"] == "sentinel-run-receipt")
if receipt_sha != expected_receipt_sha:
    raise SystemExit("receipt changed after replay integrity check")
index = {
    "schema":"ingen.sentinel-callback-auth-acceptance/v1", "status":"passed",
    "checked_at":datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z"),
    "project_root":str(root), "receipt_sha256":receipt_sha,
    "sentinel_binary":{"path":str(binary_path), "sha256":binary_sha,
                       "version_report_sha256":version_report_sha,
                       "version":{"name":version["name"], "version":version["version"],
                                  "revision":version["revision"], "goos":version["goos"],
                                  "goarch":version["goarch"], "toolchain":version["toolchain"]}},
    "checks":["valid signed batch appended one event", "exact replay left receipt bytes unchanged",
              "wrong sender rejected without receipt change", "altered payload rejected without receipt change",
              "bad HMAC rejected without receipt change", "conflicting event ID replay rejected without receipt change",
              "expired validly signed envelope rejected without receipt change", "envelope fields and digest formats match published schema"],
    "files":files,
    "limitations":["sender is a local acceptance signer, not a Herdr callback producer",
                   "HMAC proves shared-key possession only; it provides no host or application attestation",
                   "ephemeral 32-byte key was outside the project, omitted from evidence, and removed by the exit trap",
                   "synthetic project; no network or live Herdr calls"],
}
index_path.write_text(json.dumps(index, indent=2) + "\n")
PY

index_sha=$(shasum -a 256 "$project_root/$index_rel" | awk '{print $1}')
printf 'callback-auth acceptance passed\nproject: %s\nindex: %s\nindex_sha256: %s\nsentinel_sha256: %s\n' "$project_root" "$index_rel" "$index_sha" "$binary_sha"
