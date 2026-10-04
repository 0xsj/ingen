#!/usr/bin/env bash
# Runtime handoffs use only the installed directory, from an unrelated cwd.
set -euo pipefail
checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
[[ "$#" == 1 && "$1" == /* ]] || { echo 'usage: installed-tools.sh ABSOLUTE_BIN_DIRECTORY' >&2; exit 2; }
bin="$(cd "$1" && pwd -P)"
[[ "$(uname -s)" == Darwin ]] || { echo 'Managed-subject acceptance requires macOS enforcement.' >&2; exit 2; }
proof="$(mktemp -d /private/tmp/ingen-installed-tools.XXXXXX)"
trap 'printf "installed tools evidence: %s\n" "$proof"' EXIT
project="$proof/project"
mkdir -p "$project" "$proof/unrelated" "$proof/logs"
export GOCACHE="${GOCACHE:-$checkout/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$checkout/.cache/go-mod}"
# Compile the owned HTTP fixture before removing development tools from PATH.
(cd "$checkout" && go build -o "$proof/subject" ./examples/document-pipeline-lab/subject/cmd/document-pipeline)
port="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    print(sock.getsockname()[1])
PY
)"
export PATH=/usr/bin:/bin:/usr/sbin:/sbin
cd "$proof/unrelated"
"$bin/sentinel" project init --root "$project" --id document_health >"$proof/logs/init.log" 2>&1
cp "$checkout/acceptance/installed-tools/healthz.malc" "$project/.ingen/contract/spec.malc"
mkdir -p "$project/src"
mv "$proof/subject" "$project/src/subject"
# The oracle policy must declare the installed tool rather than a compiler.
sed "s|name: go|name: $bin/sorna|" "$project/.ingen/policy/oracle.yaml" >"$proof/oracle.yaml"
mv "$proof/oracle.yaml" "$project/.ingen/policy/oracle.yaml"
sed "s|ports: \[8080\]|ports: [$port]|" "$project/.ingen/policy/subject.yaml" >"$proof/subject.yaml"
mv "$proof/subject.yaml" "$project/.ingen/policy/subject.yaml"
"$bin/sentinel" contract create --root "$project" --tool-dir "$bin" >"$proof/logs/create.log" 2>&1
"$bin/sentinel" contract validate .ingen/contract/contract.json --root "$project" >"$proof/logs/validate.log" 2>&1
"$bin/sentinel" contract seal .ingen/contract/contract.json --root "$project" --output-dir .ingen/contract/sealed >"$proof/logs/seal.log" 2>&1
"$bin/sentinel" oracle freeze --root "$project" --tool-dir "$bin" >"$proof/logs/freeze.log" 2>&1
"$bin/sentinel" verify --root "$project" --tool-dir "$bin" --subject-command "$project/src/subject" --subject-arg=-addr --subject-arg "127.0.0.1:$port" --base-url "http://127.0.0.1:$port" >"$proof/logs/verify.log" 2>&1
"$bin/sentinel" evidence verify --root "$project" --tool-dir "$bin" .ingen/artifacts/evidence >"$proof/logs/evidence.log" 2>&1
"$bin/sentinel" evidence gate --root "$project" --tool-dir "$bin" .ingen/artifacts/evidence >"$proof/logs/gate.log" 2>&1
"$bin/nublar" aggregate --workflow "$project/.ingen/nublar/workflow.yaml" --root "$project" --output "$project/.ingen/artifacts/nublar-result.json" >"$proof/logs/aggregate.log" 2>&1
python3 - "$proof" "$bin" <<'PY'
import hashlib, json, pathlib, sys
root, binaries = map(pathlib.Path, sys.argv[1:])
result = json.loads((root/'project/.ingen/artifacts/nublar-result.json').read_text())
if result.get('status') != 'passed' or result.get('exit_code') != 0:
    raise SystemExit('installed workflow did not produce Nublar pass')
paths = sorted(p for p in root.rglob('*') if p.is_file() and p.name != 'subject')
index = {'schema':'ingen.installed-tools-acceptance/v1', 'status':'passed',
         'scope':'installed Malcolm/Sorna/Sentinel/Nublar handoffs on a synthetic HTTP fixture',
         'limitations':['no operator approval or native Herdr session', 'hosted CI and release publication not exercised'],
         'binaries':{name:hashlib.sha256((binaries/name).read_bytes()).hexdigest() for name in ['sentinel','sorna','sorna-malcolm','malcolm','nublar']},
         'files':{str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}}
(root/'acceptance-index.json').write_text(json.dumps(index,indent=2)+'\n')
PY
