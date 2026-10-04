#!/usr/bin/env bash
# Build, archive, and install a local bundle, then exercise installed commands.
set -euo pipefail
checkout="$(cd "$(dirname "$0")/.." && pwd -P)"
[[ "$(uname -s)" == Darwin ]] || { echo 'This installed workflow gate requires macOS enforcement.' >&2; exit 2; }
if [[ -n "${INGEN_INSTALL_PROOF_DIR:-}" ]]; then
  [[ "$INGEN_INSTALL_PROOF_DIR" = /* ]] || { echo 'INGEN_INSTALL_PROOF_DIR must be absolute.' >&2; exit 2; }
  mkdir "$INGEN_INSTALL_PROOF_DIR"
  proof="$(cd "$INGEN_INSTALL_PROOF_DIR" && pwd -P)"
else
  proof="$(mktemp -d /private/tmp/ingen-local-install.XXXXXX)"
fi
trap 'printf "local installation evidence: %s\n" "$proof"' EXIT
python="$(command -v python3)"
"$python" "$checkout/packaging/ingen_package.py" build --checkout "$checkout" --output-dir "$proof/bundle" --release-version 0.0.0-local-review >"$proof/build.log" 2>&1
digest="$(cat "$proof/bundle/manifest.sha256")"
python3 "$checkout/packaging/ingen_package.py" verify --bundle "$proof/bundle" --manifest-sha256 "$digest" >"$proof/verify.log" 2>&1
"$python" "$checkout/packaging/ingen_package.py" archive --bundle "$proof/bundle" --manifest-sha256 "$digest" --output "$proof/ingen.tar.gz" >"$proof/archive.log" 2>&1
mkdir "$proof/unrelated"
# Bootstrap from our already verified local bundle. A downloaded helper needs
# its own independently trusted digest; an untrusted helper cannot verify itself.
cp "$proof/bundle/share/ingen-package.py" "$proof/unrelated/ingen-package.py"
(
  cd "$proof/unrelated"
  export PATH=/usr/bin:/bin
  "$python" ./ingen-package.py verify-archive --archive "$proof/ingen.tar.gz" --manifest-sha256 "$digest" >"$proof/verify-archive.log" 2>&1
  "$python" ./ingen-package.py install-archive --archive "$proof/ingen.tar.gz" --manifest-sha256 "$digest" --prefix "$proof/install" >"$proof/install.log" 2>&1
  "$python" - "$proof" <<'PY'
import hashlib, json, pathlib, subprocess, sys
root = pathlib.Path(sys.argv[1])
manifest = json.loads((root/'install/manifest.json').read_text())
reports = []
for probe in manifest['version_probes']:
    report = json.loads(subprocess.check_output([str(root/'install/bin'/probe['executable']), 'version', '--format', 'json'], text=True))
    if report != probe['report']:
        raise SystemExit('installed command version differs from its manifest')
    reports.append(report)
(root/'version-reports.json').write_text(json.dumps(reports, indent=2)+'\n')
index = {'schema':'ingen.archive-install-acceptance/v1', 'status':'passed',
         'scope':'local archive verification, standalone Python installation, and nine installed version reports',
         'manifest_sha256':hashlib.sha256((root/'install/manifest.json').read_bytes()).hexdigest(),
         'target':manifest['target'],
         'source_inputs_sha256':manifest['source_inputs']['sha256'],
         'archive_sha256':hashlib.sha256((root/'ingen.tar.gz').read_bytes()).hexdigest(),
         'helper_sha256':hashlib.sha256((root/'unrelated/ingen-package.py').read_bytes()).hexdigest(),
         'version_reports_sha256':hashlib.sha256((root/'version-reports.json').read_bytes()).hexdigest(),
         'limitations':['same host with Python installed; not a clean-machine proof', 'no signature, publication, provider call, or native Herdr session']}
(root/'archive-acceptance-index.json').write_text(json.dumps(index, indent=2)+'\n')
PY
)
GOCACHE="${GO_CACHE:-$checkout/.cache/go-build}" GOMODCACHE="${GO_MOD_CACHE:-$checkout/.cache/go-mod}" INGEN_BUNDLE_VERSION_REPORTS="$proof/version-reports.json" go test ./core/cliversion/spec -run TestPublishedBundleVersionReports -count=1 >"$proof/version-schema.log" 2>&1
bash "$checkout/acceptance/installed-tools.sh" "$proof/install/bin" >"$proof/workflow.log" 2>&1
