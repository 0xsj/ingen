#!/usr/bin/env bash
# A fresh-project, nine-module HTTP workflow. The synthetic Hammond record is
# explicitly a test fixture; this does not create an operator approval.
set -euo pipefail

checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
fixture="$checkout/acceptance/ecosystem-http"
if [[ -n "${INGEN_ECOSYSTEM_HTTP_ROOT:-}" ]]; then
  proof="$INGEN_ECOSYSTEM_HTTP_ROOT"
  [[ "$proof" == /* && ! -e "$proof" ]] || { echo 'INGEN_ECOSYSTEM_HTTP_ROOT must be an absolute fresh path' >&2; exit 2; }
  mkdir -p "$proof"
else
  proof="$(mktemp -d /private/tmp/ingen-ecosystem-http.XXXXXX)"
fi
mkdir -p "$proof/bin" "$proof/logs"
receiver_pid=""
cleanup() {
  if [[ -n "$receiver_pid" ]]; then
    kill "$receiver_pid" 2>/dev/null || true
    wait "$receiver_pid" 2>/dev/null || true
  fi
  printf 'ecosystem HTTP evidence root: %s\n' "$proof"
}
trap cleanup EXIT

[[ "$(uname -s)" == Darwin ]] || { echo 'This acceptance workflow requires macOS Sorna Seatbelt enforcement.' >&2; exit 2; }

export GOCACHE="${GOCACHE:-$checkout/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$checkout/.cache/go-mod}"
cd "$checkout"

for tool in sentinel sorna sorna-go-provider hammond paddock nublar lockwood sattler; do
  case "$tool" in
    sentinel) package=./herdr-sentinel/cmd/sentinel ;;
    sorna-go-provider) package=./sorna/cmd/sorna-go-provider ;;
    *) package="./$tool/cmd/$tool" ;;
  esac
  go build -o "$proof/bin/$tool" "$package"
done
go build -o "$proof/bin/webhook-receiver" ./acceptance/ecosystem-http/webhookreceiver
cargo build --locked --manifest-path "$checkout/malcolm/Cargo.toml"

run_logged() {
  local name="$1" expected="$2" actual=0
  shift 2
  "$@" >"$proof/logs/$name.log" 2>&1 || actual=$?
  if [[ "$actual" != "$expected" ]]; then
    printf 'expected exit %s, got %s: %s\n' "$expected" "$actual" "$*" >&2
    cat "$proof/logs/$name.log" >&2
    exit 1
  fi
}

run_project_logged() {
  (cd "$project" && run_logged "$@")
}

sha256_file() {
  shasum -a 256 "$1" | cut -d ' ' -f 1
}

project="$proof/project"
mkdir -p "$project"
"$proof/bin/sentinel" project init --root "$project" --id document_flow >"$proof/logs/project-init.log"
mkdir -p "$project/examples/document-pipeline-lab" "$project/.ingen/governance" \
  "$project/.ingen/acceptance" "$project/.ingen/artifacts/checks" \
  "$project/.ingen/provenance" "$project/.ingen/artifacts"
cp "$checkout/malcolm/examples/document_flow.malc" "$project/.ingen/contract/spec.malc"
cp -R "$checkout/examples/document-pipeline-lab/subject" "$project/examples/document-pipeline-lab/subject"
cp "$fixture/document-pipeline-main.go" \
  "$project/examples/document-pipeline-lab/subject/cmd/document-pipeline/main.go"
cp "$fixture/paddock.policy.yaml" "$project/.ingen/acceptance/paddock.policy.yaml"
cp "$fixture/mutations.catalogue.yaml" "$project/.ingen/acceptance/mutations.catalogue.yaml"
cp "$fixture/nublar.workflow.yaml" "$project/.ingen/acceptance/nublar.workflow.yaml"
cp "$fixture/review-authority.test-only.json" "$project/.ingen/governance/review-authority.json"
cp "$fixture/approval-record.test-only.template.json" "$project/.ingen/governance/approval-record.template.json"
cp "$fixture/review-policy.test-only.template.json" "$project/.ingen/governance/review-policy.template.json"
cat >"$project/go.mod" <<EOF
module ingen

go 1.27.1
EOF

"$proof/bin/sentinel" contract create --root "$project" \
  --spec .ingen/contract/spec.malc --ir-output .ingen/contract/document-flow.ir.json \
  --output .ingen/contract/contract.json --ingen-root "$checkout" \
  >"$proof/logs/malcolm-contract.log" 2>&1
"$proof/bin/sentinel" contract seal .ingen/contract/contract.json --root "$project" \
  --output-dir .ingen/contract/sealed >"$proof/logs/contract-seal.log" 2>&1
contract_sha="$(cat "$project/.ingen/contract/sealed/hash.txt" | tr -d '\r\n')"
authority_sha="$(sha256_file "$project/.ingen/governance/review-authority.json")"
sed "s/@AUTHORITY_SHA256@/$authority_sha/g" "$project/.ingen/governance/review-policy.template.json" \
  >"$project/.ingen/governance/review-policy.json"
policy_sha="$(sha256_file "$project/.ingen/governance/review-policy.json")"
sed -e "s/@CONTRACT_SHA256@/$contract_sha/g" -e "s/@POLICY_SHA256@/$policy_sha/g" \
  "$project/.ingen/governance/approval-record.template.json" \
  >"$project/.ingen/governance/approval-record.json"

run_logged hammond-gate 0 "$proof/bin/hammond" gate --root "$project" \
  --approval .ingen/governance/approval-record.json \
  --review-policy .ingen/governance/review-policy.json \
  --contract .ingen/contract/sealed/canonical.json --project-id document_flow \
  --contract-id document_flow --contract-version 2 --contract-schema ingen.contract/v1 \
  --contract-sha256 "$contract_sha" --output .ingen/artifacts/checks/hammond.json

port=$((20000 + $$ % 30000))
sed "s/@PORT@/$port/g" "$fixture/subject-policy.template.yaml" \
  >"$project/.ingen/policy/subject.yaml"
"$proof/bin/sentinel" oracle freeze --root "$project" \
  --contract .ingen/contract/contract.json --policy .ingen/policy/oracle.yaml \
  --output-dir .ingen/artifacts/oracle --ingen-root "$checkout" --governed \
  --workspace .ingen/workspace.yaml --approval .ingen/governance/approval-record.json \
  --review-policy .ingen/governance/review-policy.json >"$proof/logs/oracle-freeze.log" 2>&1

subject="$project/examples/document-pipeline-lab/subject"
mkdir -p "$project/.ingen/artifacts/document-pipeline"
(cd "$project" && go build -o .ingen/artifacts/document-pipeline/document-pipeline \
  ./examples/document-pipeline-lab/subject/cmd/document-pipeline)
address="localhost:$port"

run_logged paddock-pass 0 "$proof/bin/paddock" ci "$project" \
  --policy "$project/.ingen/acceptance/paddock.policy.yaml" \
  --output "$project/.ingen/artifacts/checks/paddock.json"
"$proof/bin/paddock" map "$project" --policy "$project/.ingen/acceptance/paddock.policy.yaml" \
  --format json >"$project/.ingen/artifacts/paddock-component-map.json"
python3 - "$project/.ingen/artifacts/checks/paddock.json" \
  "$project/.ingen/artifacts/paddock-component-map.json" <<'PY'
import json, sys
ci = json.load(open(sys.argv[1], encoding="utf-8"))
result = json.load(open(sys.argv[2], encoding="utf-8"))
if ci.get("status") != "passed" or not ci.get("report") or not ci["report"].get("package_count") or not ci["report"].get("edge_count"):
    raise SystemExit("Paddock CI did not report a non-empty graph")
if not result.get("components") or not result.get("dependencies"):
    raise SystemExit("Paddock did not report a non-empty component graph")
if not any(edge.get("from") == "composition" and edge.get("to") == "document-service" for edge in result["dependencies"]):
    raise SystemExit("Paddock graph did not prove the command-to-service composition edge")
PY

"$proof/bin/sentinel" provenance start --root "$project" \
  --output .ingen/provenance/root.json >"$proof/logs/provenance-root.log" 2>&1
parent_sha="$(sha256_file "$project/.ingen/provenance/root.json")"
run_logged amber-coordinator 0 "$proof/bin/sentinel" provenance execute --root "$project" \
  --parent .ingen/provenance/root.json --output .ingen/provenance/sorna-child.json \
  --receipt .ingen/provenance/sorna-execution.json --operation sorna-managed-http-verification \
  --expected-parent-sha256 "$parent_sha" -- "$proof/bin/sentinel" verify \
  --root "$project" --ingen-root "$checkout" --oracle .ingen/artifacts/oracle/oracle.json \
  --policy .ingen/policy/oracle.yaml --subject-policy .ingen/policy/subject.yaml \
  --subject-root . --subject-dir . --base-url "http://$address" --ready-path /healthz \
  --subject-variant ecosystem-http-clean \
  --subject-command "$project/.ingen/artifacts/document-pipeline/document-pipeline" \
  --subject-arg=-addr --subject-arg "127.0.0.1:$port" --output-dir .ingen/artifacts/evidence
run_logged sora-behavior-ci 0 "$proof/bin/sentinel" evidence gate --root "$project" \
  --ingen-root "$checkout" --output .ingen/artifacts/checks/behavior.json \
  .ingen/artifacts/evidence

run_logged mutation-plan 0 "$proof/bin/sorna" mutation plan \
  "$project/.ingen/acceptance/mutations.catalogue.yaml" \
  --contract "$project/.ingen/contract/contract.json" \
  --oracle "$project/.ingen/artifacts/oracle/oracle.json" \
  --baseline-evidence "$project/.ingen/artifacts/evidence" \
  --subject-policy "$project/.ingen/policy/subject.yaml" \
  --output "$project/.ingen/artifacts/mutation-plan.json"
run_logged mutation-provider-build 0 "$proof/bin/sorna-go-provider" \
  --subject document-pipeline --plan "$project/.ingen/artifacts/mutation-plan.json" \
  --source-root "$project" --output-dir "$project/.ingen/artifacts/go-provider" \
  --binary-dir "$project/.ingen/artifacts/go-mutations" \
  --provider "$project/.ingen/artifacts/go-provider/provider.yaml" \
  --summary-output "$project/.ingen/artifacts/go-provider/preparation.json"
run_logged mutation-provider-review 0 "$proof/bin/sorna" mutation provider inspect \
  "$project/.ingen/artifacts/mutation-plan.json" \
  --provider "$project/.ingen/artifacts/go-provider/provider.yaml" --require-plan-binding \
  --format ci-result --output "$project/.ingen/artifacts/checks/mutation-provider.json"
run_logged mutation-preparation-review 0 "$proof/bin/sorna" mutation provider preparation \
  "$project/.ingen/artifacts/go-provider/preparation.json" \
  --provider "$project/.ingen/artifacts/go-provider/provider.yaml" --source-root "$project" \
  --format ci-result --output "$project/.ingen/artifacts/checks/mutation-preparation.json"
run_project_logged mutation-campaign-run 0 "$proof/bin/sorna" mutation run \
  "$project/.ingen/artifacts/mutation-plan.json" \
  --provider "$project/.ingen/artifacts/go-provider/provider.yaml" --require-plan-binding \
  --oracle "$project/.ingen/artifacts/oracle/oracle.json" \
  --policy "$project/.ingen/policy/oracle.yaml" \
  --subject-policy "$project/.ingen/policy/subject.yaml" --base-address "$address" \
  --output-dir "$project/.ingen/artifacts/mutation-campaign" \
  --output "$project/.ingen/artifacts/mutation-campaign/campaign-result.json"
run_logged mutation-campaign-verify 0 "$proof/bin/sorna" mutation verify \
  "$project/.ingen/artifacts/mutation-campaign/campaign-result.json" \
  --format ci-result --source-root "$project" \
  --output "$project/.ingen/artifacts/checks/mutation.json"

lockwood_store="$project/.ingen/artifacts/lockwood"
mkdir -p "$lockwood_store"
put_record() {
  local id="$1" producer="$2" kind="$3" source="$4" media="${5:-application/json}" relation="${6:-}" actual_digest
  actual_digest="$(sha256_file "$source")"
  local parents=()
  if [[ -n "$relation" ]]; then parents=(--parent "$relation"); fi
  run_logged "lockwood-put-$id" 0 "$proof/bin/lockwood" put --root "$lockwood_store" \
    --id "$id" --media-type "$media" --producer "$producer" --kind "$kind" \
    --name "$(basename "$source")" --expected-digest "sha256:$actual_digest" \
    --source-path "$source" "${parents[@]}" "$source"
}

put_record ecosystem-contract sorna malcolm-translated-contract "$project/.ingen/contract/contract.json"
put_record ecosystem-canonical-contract sorna sealed-contract "$project/.ingen/contract/sealed/canonical.json"
authority_sha="$(sha256_file "$project/.ingen/governance/review-authority.json")"
put_record ecosystem-review-authority hammond review-authority "$project/.ingen/governance/review-authority.json"
put_record ecosystem-review-policy hammond active-review-policy "$project/.ingen/governance/review-policy.json" \
  application/json "references=sha256:$authority_sha"
put_record ecosystem-approval-record hammond approved-contract-record "$project/.ingen/governance/approval-record.json" \
  application/json "references=sha256:$contract_sha"
put_record ecosystem-oracle-policy sorna oracle-policy "$project/.ingen/policy/oracle.yaml" text/plain
put_record ecosystem-frozen-oracle sorna frozen-oracle "$project/.ingen/artifacts/oracle/oracle.json" \
  application/json "references=sha256:$contract_sha"
put_record ecosystem-amber-root amber provenance-context "$project/.ingen/provenance/root.json" \
  application/json "references=sha256:$contract_sha"
put_record ecosystem-amber-child amber provenance-context "$project/.ingen/provenance/sorna-child.json" \
  application/json "references=sha256:$(sha256_file "$project/.ingen/provenance/root.json")"
for capture in .ingen/provenance/sorna-execution.json .ingen/provenance/sorna-execution.json.stdout .ingen/provenance/sorna-execution.json.stderr; do
  media=application/json
  [[ "$capture" != *.stdout && "$capture" != *.stderr ]] || media=text/plain
  put_record "ecosystem-sentinel-$(basename "$capture" | tr '. ' '--')" sentinel provenance-evidence \
    "$project/$capture" "$media" "references=sha256:$(sha256_file "$project/.ingen/provenance/sorna-child.json")"
done
run_logged lockwood-import-sorna-evidence 0 "$proof/bin/lockwood" import-sorna \
  --root "$lockwood_store" --id ecosystem-sorna-evidence-bundle-clean \
  "$project/.ingen/artifacts/evidence"
put_record ecosystem-sorna-run-clean sorna behavioral-run "$project/.ingen/artifacts/evidence/run.json"

for item in hammond paddock behavior mutation; do
  digest="$(sha256_file "$project/.ingen/artifacts/checks/$item.json")"
  producer="$item"
  kind=ci-result
  case "$item" in
    behavior) producer=sorna; kind=behavioral-verification ;;
    mutation) producer=sorna; kind=mutation-campaign-verification ;;
  esac
  parent_arg=()
  if [[ "$item" != hammond ]]; then
    parent_digest="$(sha256_file "$project/.ingen/artifacts/checks/hammond.json")"
    parent_arg=(--parent "references=sha256:$parent_digest")
  fi
  run_logged "lockwood-put-$item" 0 "$proof/bin/lockwood" put --root "$lockwood_store" \
    --id "ecosystem-$item" --media-type application/json --producer "$producer" \
    --kind "$kind" --name "$item.json" --expected-digest "sha256:$digest" \
    --source-path ".ingen/artifacts/checks/$item.json" "${parent_arg[@]}" \
    "$project/.ingen/artifacts/checks/$item.json"
done
run_logged custody-extra-before 0 python3 "$fixture/custody-extra.py" \
  --lockwood "$proof/bin/lockwood" --project "$project" --phase before \
  --store "$lockwood_store" --index .ingen/artifacts/checks/lockwood-extra-before.json
extra_before_args=()
while IFS= read -r extra_id; do extra_before_args+=(--id "$extra_id"); done < <(
  python3 -c 'import json,sys; print("\n".join(json.load(open(sys.argv[1]))["ids"]))' \
    "$project/.ingen/artifacts/checks/lockwood-extra-before.json"
)
base_before_args=(
  --id ecosystem-hammond
  --id ecosystem-paddock
  --id ecosystem-behavior
  --id ecosystem-mutation
  --id ecosystem-contract
  --id ecosystem-canonical-contract
  --id ecosystem-review-authority
  --id ecosystem-review-policy
  --id ecosystem-approval-record
  --id ecosystem-oracle-policy
  --id ecosystem-frozen-oracle
  --id ecosystem-amber-root
  --id ecosystem-amber-child
  --id ecosystem-sorna-evidence-bundle-clean
  --id ecosystem-sorna-run-clean
  --id ecosystem-sentinel-sorna-execution-json
  --id ecosystem-sentinel-sorna-execution-json-stdout
  --id ecosystem-sentinel-sorna-execution-json-stderr
)
run_logged lockwood-ci 0 "$proof/bin/lockwood" ci-result --root "$lockwood_store" \
  "${base_before_args[@]}" \
  "${extra_before_args[@]}" \
  --output "$project/.ingen/artifacts/checks/lockwood.json"

workflow="$project/.ingen/acceptance/nublar.workflow.yaml"
run_logged nublar-clean-collect 0 "$proof/bin/nublar" run collect --workflow "$workflow" \
  --root "$project" --store "$project/.ingen/artifacts/nublar-runs" \
  --run-id ecosystem-clean --external-system ecosystem-http --external-id document-flow --attempt 1 \
  --output "$project/.ingen/artifacts/nublar-clean.json"
run_logged nublar-clean-decision 0 "$proof/bin/nublar" run decision \
  --store "$project/.ingen/artifacts/nublar-runs" --run-id ecosystem-clean \
  --output "$project/.ingen/artifacts/nublar-clean-decision.json"
cp "$project/.ingen/artifacts/checks/behavior.json" "$project/.ingen/artifacts/checks/behavior-clean.json"
run_logged lockwood-custody-clean 0 "$proof/bin/lockwood" inspect --root "$lockwood_store" \
  ecosystem-behavior
cp "$proof/logs/lockwood-custody-clean.log" "$project/.ingen/artifacts/lockwood-custody-clean.json"
mv "$project/.ingen/artifacts/checks/lockwood.json" "$project/.ingen/artifacts/checks/lockwood-clean.json"

python3 - "$subject/server.go" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
text = path.read_text()
before = 'writeJSONWithEvents(w, http.StatusAccepted, map[string]string{'
after = 'writeJSONWithEvents(w, http.StatusOK, map[string]string{'
if text.count(before) != 1:
    raise SystemExit("expected exactly one accepted-status response to mutate")
path.write_text(text.replace(before, after, 1))
PY
(cd "$project" && go build -o .ingen/artifacts/document-pipeline/document-pipeline-defect \
  ./examples/document-pipeline-lab/subject/cmd/document-pipeline)
run_logged amber-provenance-failed-variant 1 "$proof/bin/sentinel" provenance execute --root "$project" \
  --parent .ingen/provenance/root.json --output .ingen/provenance/sorna-defect-child.json \
  --receipt .ingen/provenance/sorna-defect-execution.json --operation sorna-managed-http-defect-verification \
  --expected-parent-sha256 "$parent_sha" -- "$proof/bin/sentinel" verify \
  --root "$project" --ingen-root "$checkout" --oracle .ingen/artifacts/oracle/oracle.json \
  --policy .ingen/policy/oracle.yaml --subject-policy .ingen/policy/subject.yaml \
  --subject-root . --subject-dir . --base-url "http://$address" --ready-path /healthz \
  --subject-variant ecosystem-http-status-200-defect \
  --subject-command "$project/.ingen/artifacts/document-pipeline/document-pipeline-defect" \
  --subject-arg=-addr --subject-arg "127.0.0.1:$port" --output-dir .ingen/artifacts/evidence-defect
run_logged sora-behavior-defect-ci 1 "$proof/bin/sentinel" evidence gate --root "$project" \
  --ingen-root "$checkout" --output .ingen/artifacts/checks/behavior.json --force \
  .ingen/artifacts/evidence-defect

digest="$(sha256_file "$project/.ingen/artifacts/checks/behavior.json")"
parent_digest="$(sha256_file "$project/.ingen/artifacts/checks/hammond.json")"
run_logged lockwood-put-behavior-defect 0 "$proof/bin/lockwood" put --root "$lockwood_store" \
  --id ecosystem-behavior-defect --media-type application/json --producer sorna \
  --kind behavioral-verification --name behavior-defect.json --expected-digest "sha256:$digest" \
  --source-path .ingen/artifacts/checks/behavior.json \
  --parent "references=sha256:$parent_digest" "$project/.ingen/artifacts/checks/behavior.json"
run_logged custody-extra-after 0 python3 "$fixture/custody-extra.py" \
  --lockwood "$proof/bin/lockwood" --project "$project" --phase after \
  --store "$lockwood_store" --index .ingen/artifacts/checks/lockwood-extra-after.json
extra_after_args=()
while IFS= read -r extra_id; do extra_after_args+=(--id "$extra_id"); done < <(
  python3 -c 'import json,sys; print("\n".join(json.load(open(sys.argv[1]))["ids"]))' \
    "$project/.ingen/artifacts/checks/lockwood-extra-after.json"
)
run_logged lockwood-defect-ci 0 "$proof/bin/lockwood" ci-result --root "$lockwood_store" \
  --id ecosystem-hammond --id ecosystem-paddock --id ecosystem-behavior-defect --id ecosystem-mutation \
  "${extra_before_args[@]}" "${extra_after_args[@]}" \
  --output "$project/.ingen/artifacts/checks/lockwood.json"
run_logged lockwood-custody-defect 0 "$proof/bin/lockwood" inspect --root "$lockwood_store" \
  ecosystem-behavior-defect
cp "$proof/logs/lockwood-custody-defect.log" "$project/.ingen/artifacts/lockwood-custody-defect.json"
run_logged nublar-defect-collect 1 "$proof/bin/nublar" run collect --workflow "$workflow" \
  --root "$project" --store "$project/.ingen/artifacts/nublar-runs" \
  --run-id ecosystem-defect --external-system ecosystem-http --external-id document-flow --attempt 2 \
  --output "$project/.ingen/artifacts/nublar-defect.json"
run_logged nublar-defect-decision 0 "$proof/bin/nublar" run decision \
  --store "$project/.ingen/artifacts/nublar-runs" --run-id ecosystem-defect \
  --output "$project/.ingen/artifacts/nublar-defect-decision.json"

"$proof/bin/webhook-receiver" "$proof/webhook.port" "$proof/webhook.request.json" \
  >"$proof/logs/webhook-receiver.log" 2>&1 &
receiver_pid=$!
for _ in $(seq 1 100); do
  [[ -s "$proof/webhook.port" ]] && break
  sleep 0.05
done
[[ -s "$proof/webhook.port" ]]
webhook_port="$(cat "$proof/webhook.port" | tr -d '\r\n')"
run_logged nublar-webhook-delivery 0 "$proof/bin/nublar" run deliver \
  --transport http-webhook --store "$project/.ingen/artifacts/nublar-runs" \
  --run-id ecosystem-defect --webhook "http://127.0.0.1:$webhook_port/" \
  --receipt "$project/.ingen/artifacts/nublar-defect-delivery.json" \
  --receipt-store "$project/.ingen/artifacts/nublar-delivery-receipts"
wait "$receiver_pid"
receiver_pid=""
run_logged custody-extra-delivery 0 python3 "$fixture/custody-extra.py" \
  --lockwood "$proof/bin/lockwood" --project "$project" --phase delivery \
  --store "$lockwood_store" --webhook-json "$proof/webhook.request.json" \
  --index .ingen/artifacts/checks/lockwood-extra-delivery.json
extra_delivery_args=()
while IFS= read -r extra_id; do extra_delivery_args+=(--id "$extra_id"); done < <(
  python3 -c 'import json,sys; print("\n".join(json.load(open(sys.argv[1]))["ids"]))' \
    "$project/.ingen/artifacts/checks/lockwood-extra-delivery.json"
)
run_logged lockwood-complete-ci 0 "$proof/bin/lockwood" ci-result --root "$lockwood_store" \
  "${base_before_args[@]}" --id ecosystem-behavior-defect \
  "${extra_before_args[@]}" "${extra_after_args[@]}" "${extra_delivery_args[@]}" \
  --output "$project/.ingen/artifacts/checks/lockwood-complete.json"

mkdir -p "$project/.ingen/artifacts/sattler"
cat >"$project/.ingen/artifacts/sattler/before-after.json" <<'JSON'
{
  "schema": "ingen.sattler-comparison-input/v0",
  "before": {
    "ci_result": "../checks/behavior-clean.json",
    "sorna_run": "../evidence/run.json",
    "nublar_run": "../nublar-clean.json",
    "custody": "../lockwood-custody-clean.json",
    "provenance": "../../provenance/sorna-child.json"
  },
  "after": {
    "ci_result": "../checks/behavior.json",
    "sorna_run": "../evidence-defect/run.json",
    "nublar_run": "../nublar-defect.json",
    "custody": "../lockwood-custody-defect.json",
    "provenance": "../../provenance/sorna-defect-child.json"
  }
}
JSON
run_logged sattler-before-after 0 "$proof/bin/sattler" bundle compare --format json \
  --output "$project/.ingen/artifacts/sattler/before-after-result.json" \
  "$project/.ingen/artifacts/sattler/before-after.json"

# Missing governance must produce an explicit Hammond rejection envelope.
mkdir -p "$project/.ingen/artifacts/negative"
run_logged missing-governance 2 "$proof/bin/hammond" gate --root "$project" \
  --approval .ingen/governance/missing-approval.json \
  --review-policy .ingen/governance/review-policy.json \
  --contract .ingen/contract/sealed/canonical.json --project-id document_flow \
  --contract-id document_flow --contract-version 2 --contract-schema ingen.contract/v1 \
  --contract-sha256 "$contract_sha" --output .ingen/artifacts/negative/missing-governance.json

# Add one compilable service -> composition import and require Paddock to reject it.
mkdir -p "$subject/cmd/document-pipeline/forbiddenfixture"
cat >"$subject/cmd/document-pipeline/forbiddenfixture/fixture.go" <<'GO'
package forbiddenfixture

const Name = "architecture negative fixture"
GO
cat >"$subject/architecture_violation.go" <<'GO'
package documentpipeline

import _ "ingen/examples/document-pipeline-lab/subject/cmd/document-pipeline/forbiddenfixture"
GO
run_logged paddock-forbidden-edge 1 "$proof/bin/paddock" ci "$project" \
  --policy "$project/.ingen/acceptance/paddock.policy.yaml" \
  --output "$project/.ingen/artifacts/negative/paddock-forbidden-edge.json"

# Damage a real accepted parent record after importing a child that references it.
printf 'lineage child evidence\n' >"$proof/lineage-child.txt"
lineage_parent_sha="$(sha256_file "$project/.ingen/artifacts/checks/hammond.json")"
run_logged lockwood-lineage-child 0 "$proof/bin/lockwood" put --root "$lockwood_store" \
  --id ecosystem-lineage-child --media-type text/plain --producer acceptance \
  --kind lineage-test --name lineage-child.txt \
  --parent "derived-from=sha256:$lineage_parent_sha" "$proof/lineage-child.txt"
parent_blob="$lockwood_store/blobs/sha256/${lineage_parent_sha:0:2}/${lineage_parent_sha:2:2}/$lineage_parent_sha"
cp "$parent_blob" "$proof/parent-blob.original"
printf 'damaged custody parent blob\n' >"$parent_blob"
cp "$parent_blob" "$proof/damaged-parent-blob"
run_logged lockwood-damaged-parent 1 "$proof/bin/lockwood" ci-result --root "$lockwood_store" \
  --id ecosystem-lineage-child --output "$project/.ingen/artifacts/negative/lockwood-damaged-parent.json"
mv "$proof/parent-blob.original" "$parent_blob"

# A missing required input is an orchestration error; restore the input afterward.
mv "$project/.ingen/artifacts/checks/mutation.json" "$project/.ingen/artifacts/checks/mutation.json.saved"
run_logged nublar-missing-required 2 "$proof/bin/nublar" run collect --workflow "$workflow" \
  --root "$project" --store "$project/.ingen/artifacts/nublar-runs" \
  --run-id ecosystem-missing-evidence --output "$project/.ingen/artifacts/negative/nublar-missing-evidence.json"
mv "$project/.ingen/artifacts/checks/mutation.json.saved" "$project/.ingen/artifacts/checks/mutation.json"

python3 - "$project" "$proof" <<'PY'
import json, pathlib, sys
project, proof = map(pathlib.Path, sys.argv[1:])
def load(path):
    return json.loads(path.read_text(encoding="utf-8"))
clean = load(project / ".ingen/artifacts/nublar-clean.json")
failed = load(project / ".ingen/artifacts/nublar-defect.json")
decision = load(project / ".ingen/artifacts/nublar-defect-decision.json")
delivery = load(project / ".ingen/artifacts/nublar-defect-delivery.json")
comparison = load(project / ".ingen/artifacts/sattler/before-after-result.json")
custody_complete = load(project / ".ingen/artifacts/checks/lockwood-complete.json")
provider_review = load(project / ".ingen/artifacts/checks/mutation-provider.json")
preparation_review = load(project / ".ingen/artifacts/checks/mutation-preparation.json")
campaign = load(project / ".ingen/artifacts/mutation-campaign/campaign-result.json")
if clean.get("status") != "passed" or clean.get("exit_code") != 0:
    raise SystemExit("Nublar did not record a passing clean workflow run")
if failed.get("status") != "failed" or failed.get("exit_code") != 1 or decision.get("status") != "failed" or decision.get("exit_code") != 1:
    raise SystemExit("Nublar did not preserve clean versus behavioral-defect outcomes")
if delivery.get("status") not in ("accepted", "delivered", "passed"):
    raise SystemExit("Nublar did not record local delivery acceptance")
if provider_review.get("status") != "passed" or provider_review.get("exit_code") != 0:
    raise SystemExit("Sorna mutation provider review did not pass")
if preparation_review.get("status") != "passed" or preparation_review.get("exit_code") != 0:
    raise SystemExit("Sorna mutation preparation review did not pass")
summary = campaign.get("summary", {})
entries = campaign.get("entries", [])
if campaign.get("status") != "passed" or summary.get("total") != 1 or summary.get("killed") != 1 or len(entries) != 1:
    raise SystemExit("the real generated mutation campaign did not kill exactly one mutation")
mutation = entries[0]
if mutation.get("outcome") != "killed" or mutation.get("status") != "passed" or "create_document.requirement.1" not in mutation.get("diagnosis", {}).get("directly_failed_rules", []):
    raise SystemExit("the generated HTTP 202-to-200 mutation was not killed by its target rule")
subsystems = comparison.get("summary", {}).get("subsystems", {})
for name in ("ci_result", "sorna_run", "nublar_run", "custody"):
    if not subsystems.get(name, {}).get("compatible"):
        raise SystemExit(f"Sattler could not compare the {name} before/after artifacts")
if subsystems["ci_result"].get("transition", {}).get("before") != "passed" or subsystems["ci_result"].get("transition", {}).get("after") != "failed":
    raise SystemExit("Sattler did not preserve the behavioral CI pass-to-fail transition")
if subsystems["nublar_run"].get("transition", {}).get("before") != "passed" or subsystems["nublar_run"].get("transition", {}).get("after") != "failed":
    raise SystemExit("Sattler did not preserve the Nublar pass-to-fail decision transition")
if subsystems["sorna_run"].get("transition", {}).get("before") != "pass" or subsystems["sorna_run"].get("transition", {}).get("after") != "fail":
    raise SystemExit("Sattler did not preserve the Sorna pass-to-fail verdict transition")
reasons = comparison.get("summary", {}).get("compatibility_reasons", [])
if comparison.get("summary", {}).get("compatible") is not True:
    import re
    expected_work_id_change = re.compile(r'^provenance: work id changed from "[0-9a-f-]{36}" to "[0-9a-f-]{36}"$')
    if not reasons or any(not expected_work_id_change.fullmatch(reason) for reason in reasons):
        raise SystemExit("Sattler reported a comparison incompatibility beyond distinct Amber work IDs")
if custody_complete.get("status") != "passed" or custody_complete.get("exit_code") != 0:
    raise SystemExit("Lockwood did not verify the full before/after/delivery custody intake")
verification = custody_complete.get("report", {}).get("verification", {})
if verification.get("checked", 0) <= 0 or verification.get("checked") != verification.get("verified") or verification.get("failed") != 0:
    raise SystemExit("Lockwood full custody counts do not agree with a complete verification")
base_ids = {
    "ecosystem-hammond", "ecosystem-paddock", "ecosystem-behavior", "ecosystem-mutation",
    "ecosystem-contract", "ecosystem-canonical-contract", "ecosystem-review-authority",
    "ecosystem-review-policy", "ecosystem-approval-record", "ecosystem-oracle-policy",
    "ecosystem-frozen-oracle", "ecosystem-amber-root", "ecosystem-amber-child",
    "ecosystem-sorna-evidence-bundle-clean", "ecosystem-sorna-run-clean",
    "ecosystem-sentinel-sorna-execution-json", "ecosystem-sentinel-sorna-execution-json-stdout",
    "ecosystem-sentinel-sorna-execution-json-stderr", "ecosystem-behavior-defect",
}
expected_ids = set(base_ids)
for phase in ("before", "after", "delivery"):
    index = load(project / f".ingen/artifacts/checks/lockwood-extra-{phase}.json")
    if index.get("phase") != phase or not isinstance(index.get("ids"), list):
        raise SystemExit(f"Lockwood {phase} custody index is invalid")
    expected_ids.update(index["ids"])
selected = custody_complete.get("report", {}).get("selected_custody_ids", [])
if len(selected) != len(expected_ids) or set(selected) != expected_ids:
    raise SystemExit("Lockwood final CI result did not select the full baseline, defect, and delivery custody union")
if verification.get("checked") != len(expected_ids):
    raise SystemExit("Lockwood verification count does not match the complete selected custody union")
body = json.loads((proof / "webhook.request.json").read_text(encoding="utf-8"))
if body.get("run_id") != "ecosystem-defect":
    raise SystemExit("local webhook did not receive the immutable failed Nublar run ID")
missing = load(project / ".ingen/artifacts/negative/missing-governance.json")
bad_paddock = load(project / ".ingen/artifacts/negative/paddock-forbidden-edge.json")
bad_lockwood = load(project / ".ingen/artifacts/negative/lockwood-damaged-parent.json")
missing_nublar = load(project / ".ingen/artifacts/negative/nublar-missing-evidence.json")
if missing.get("status") != "error" or missing.get("exit_code") != 2:
    raise SystemExit("missing governance did not produce Hammond error/2")
if bad_paddock.get("status") != "failed" or bad_paddock.get("exit_code") != 1:
    raise SystemExit("forbidden architecture edge did not produce Paddock failed/1")
if bad_lockwood.get("status") != "failed" or bad_lockwood.get("exit_code") != 1:
    raise SystemExit("damaged custody parent did not produce Lockwood failed/1")
if missing_nublar.get("status") != "error" or missing_nublar.get("exit_code") != 2:
    raise SystemExit("missing required Nublar evidence did not produce error/2")
PY

python3 - "$checkout" "$project" "$proof" <<'PY'
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import tempfile
import sys

checkout, project, proof = map(Path, sys.argv[1:])
def load(path):
    return json.loads(path.read_text(encoding="utf-8"))
def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()
def proof_ref(path):
    return {"path": path.relative_to(proof).as_posix(), "sha256": digest(path)}
def ci_status(label, relative):
    path = project / relative
    artifact = load(path)
    return {"path": path.relative_to(proof).as_posix(), "tool": artifact.get("tool"),
            "kind": artifact.get("kind"), "status": artifact.get("status"),
            "exit_code": artifact.get("exit_code"), "sha256": digest(path)}

ci_paths = {
    "hammond_approval": ".ingen/artifacts/checks/hammond.json",
    "paddock_architecture": ".ingen/artifacts/checks/paddock.json",
    "sorna_behavior_clean": ".ingen/artifacts/checks/behavior-clean.json",
    "sorna_mutation_provider": ".ingen/artifacts/checks/mutation-provider.json",
    "sorna_mutation_preparation": ".ingen/artifacts/checks/mutation-preparation.json",
    "sorna_mutation_campaign": ".ingen/artifacts/checks/mutation.json",
    "sorna_behavior_defect": ".ingen/artifacts/checks/behavior.json",
    "lockwood_clean": ".ingen/artifacts/checks/lockwood-clean.json",
    "lockwood_defect": ".ingen/artifacts/checks/lockwood.json",
    "lockwood_complete": ".ingen/artifacts/checks/lockwood-complete.json",
    "hammond_missing_governance": ".ingen/artifacts/negative/missing-governance.json",
    "paddock_forbidden_edge": ".ingen/artifacts/negative/paddock-forbidden-edge.json",
    "lockwood_damaged_parent": ".ingen/artifacts/negative/lockwood-damaged-parent.json",
    "nublar_missing_required": ".ingen/artifacts/negative/nublar-missing-evidence.json",
}
ci = {label: ci_status(label, relative) for label, relative in ci_paths.items()}
paddock = load(project / ci_paths["paddock_architecture"]).get("report", {})
campaign = load(project / ".ingen/artifacts/mutation-campaign/campaign-result.json")
lockwood = load(project / ci_paths["lockwood_complete"]).get("report", {}).get("verification", {})
clean_run = load(project / ".ingen/artifacts/nublar-clean.json")
defect_run = load(project / ".ingen/artifacts/nublar-defect.json")
clean_decision = load(project / ".ingen/artifacts/nublar-clean-decision.json")
defect_decision = load(project / ".ingen/artifacts/nublar-defect-decision.json")
delivery = load(project / ".ingen/artifacts/nublar-defect-delivery.json")
comparison = load(project / ".ingen/artifacts/sattler/before-after-result.json")
subject_policy_path = project / ".ingen/policy/subject.yaml"
provenance_receipt = load(project / ".ingen/provenance/sorna-execution.json")
provenance_context = load(project / ".ingen/provenance/sorna-child.json")
contract_hash_path = project / ".ingen/contract/sealed/hash.txt"
canonical_path = project / ".ingen/contract/sealed/canonical.json"
oracle_path = project / ".ingen/artifacts/oracle/oracle.json"
oracle_manifest = load(project / ".ingen/artifacts/oracle/manifest.json")
oracle_run = load(project / ".ingen/artifacts/evidence/run.json")
malcolm_binary = checkout / "malcolm/target/debug/malcolm"
tool_paths = {path.name: path for path in sorted((proof / "bin").iterdir()) if path.is_file()}
if not malcolm_binary.is_file():
    raise SystemExit(f"Malcolm CLI binary missing after locked build: {malcolm_binary}")
tool_paths["malcolm"] = malcolm_binary
tool_hashes = {name: {"path": str(path), "sha256": digest(path)} for name, path in sorted(tool_paths.items())}

missing_check = load(project / ci_paths["nublar_missing_required"])
approval = load(project / ".ingen/governance/approval-record.json")
authority = load(project / ".ingen/governance/review-authority.json")
index = {
    "schema": "ingen.ecosystem-http-acceptance-index/v1",
    "created_at": datetime.now(timezone.utc).isoformat(),
    "evidence_root": str(proof),
    "project_root": str(project),
    "contract": {
        "id": "document_flow", "version": 2,
        "canonical_identity_sha256": contract_hash_path.read_text(encoding="utf-8").strip(),
        "canonical_file": proof_ref(canonical_path),
        "source_spec": proof_ref(project / ".ingen/contract/spec.malc"),
    },
    "oracle": {
        "identity_sha256": oracle_manifest["oracle"]["sha256"],
        "artifact_file": proof_ref(oracle_path),
        "producer_assurance": oracle_manifest.get("assurance", {}),
    },
    "cli_binaries": tool_hashes,
    "ci_results": ci,
    "counts": {
        "paddock": {"packages": paddock.get("package_count"), "edges": paddock.get("edge_count")},
        "mutation_campaign": campaign.get("summary", {}),
        "lockwood_complete": {"checked": lockwood.get("checked"), "verified": lockwood.get("verified"), "failed": lockwood.get("failed")},
        "nublar": {"clean_checks": len(clean_run.get("checks", [])), "defect_checks": len(defect_run.get("checks", []))},
    },
    "nublar": {
        "clean_run": {"status": clean_run.get("status"), "exit_code": clean_run.get("exit_code"), "path": proof_ref(project / ".ingen/artifacts/nublar-clean.json")},
        "clean_decision": {"status": clean_decision.get("status"), "exit_code": clean_decision.get("exit_code"), "path": proof_ref(project / ".ingen/artifacts/nublar-clean-decision.json")},
        "defect_run": {"status": defect_run.get("status"), "exit_code": defect_run.get("exit_code"), "path": proof_ref(project / ".ingen/artifacts/nublar-defect.json")},
        "defect_decision": {"status": defect_decision.get("status"), "exit_code": defect_decision.get("exit_code"), "path": proof_ref(project / ".ingen/artifacts/nublar-defect-decision.json")},
        "local_delivery": {"status": delivery.get("status"), "path": proof_ref(project / ".ingen/artifacts/nublar-defect-delivery.json"), "webhook_request": proof_ref(proof / "webhook.request.json")},
    },
    "sattler": {
        "overall_compatible": comparison.get("summary", {}).get("compatible"),
        "compatibility_reasons": comparison.get("summary", {}).get("compatibility_reasons", []),
        "subsystems": comparison.get("summary", {}).get("subsystems", {}),
        "report": proof_ref(project / ".ingen/artifacts/sattler/before-after-result.json"),
    },
    "governance_fixture": {
        "approval_state": approval.get("state"), "review_authority": authority.get("id"),
        "explicitly_synthetic_test_only": True, "operator_authorization": False,
    },
    "limitations": {
        "coordinator_enforcement": provenance_receipt.get("enforcement"),
        "coordinator_assurance": provenance_receipt.get("assurance"),
        "coordinator_limitations": provenance_receipt.get("limitations", []),
        "amber_context_mode": provenance_context.get("mode", {}).get("kind") if isinstance(provenance_context.get("mode"), dict) else provenance_context.get("mode"),
        "http_context_propagation_observed": False,
        "http_context_propagation_note": "The Amber context records Sentinel coordinator lineage; the HTTP subject does not consume or attest to that context.",
        "approval_note": "The Hammond approval is an explicit synthetic checked-in test fixture, not an operator approval.",
        "paddock_policy_note": "Architecture policy is acceptance-only and unsealed; it is not the production Paddock policy.",
        "native_herdr_gate": "Not exercised by this HTTP workflow.",
        "subject_policy": {
            "file": proof_ref(subject_policy_path),
            "enforcement": oracle_run.get("lifecycle", {}).get("sandbox", {}).get("enforcement"),
            "policy_sha256": oracle_run.get("lifecycle", {}).get("sandbox", {}).get("policy_sha256"),
        },
        "nublar_delivery": "Local loopback receiver only.",
        "missing_evidence_error": {"status": missing_check.get("status"), "exit_code": missing_check.get("exit_code")},
    },
}
encoded = (json.dumps(index, indent=2, sort_keys=True) + "\n").encode("utf-8")
final_path = proof / "acceptance-index.json"
if final_path.exists() or final_path.is_symlink():
    raise SystemExit(f"acceptance index already exists: {final_path}")
fd, temporary = tempfile.mkstemp(prefix=".acceptance-index.", dir=proof)
try:
    with os.fdopen(fd, "wb") as stream:
        stream.write(encoded)
        stream.flush()
        os.fsync(stream.fileno())
    os.link(temporary, final_path)
    directory = os.open(proof, os.O_RDONLY)
    try:
        os.fsync(directory)
    finally:
        os.close(directory)
finally:
    try:
        os.unlink(temporary)
    except FileNotFoundError:
        pass
PY

cat >"$proof/SUMMARY.txt" <<EOF
Workflow: ecosystem-document-flow-http (Nublar)
Subject: Malcolm document_flow v2 -> Sorna managed HTTP document pipeline
Hammond: explicit synthetic approval fixture (not operator approval)
Paddock: declaration-only test policy over the same HTTP subject source
Amber: Sentinel coordinator root/child lineage around Sorna managed verification
Lockwood: exact CI-result custody with accepted lineage verification
Nublar: immutable clean and failed runs; local webhook delivery receipt
Sattler: before/after subsystem comparison (distinct Amber work IDs remain explicit)
Negative evidence: missing governance, forbidden architecture edge, damaged custody parent, missing required producer input
Evidence root: $proof
JSON evidence index: $proof/acceptance-index.json
EOF
cat "$proof/SUMMARY.txt"
