#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: acceptance/github-checks.sh --bin ABS_NUBLAR --ecosystem-proof ABS_PROOF [--output-root ABS_NONEXISTENT]

Delivers existing clean and failed Nublar decisions to a synthetic loopback
GitHub Checks API. No remote GitHub API or real credential is used.
USAGE
}
fail() { printf 'github-checks acceptance: %s\n' "$1" >&2; exit 2; }

nublar=
ecosystem_proof=
output_root=
while (($#)); do
  case "$1" in
    --bin) (($# >= 2)) || fail "missing value for $1"; nublar=$2; shift 2 ;;
    --ecosystem-proof) (($# >= 2)) || fail "missing value for $1"; ecosystem_proof=$2; shift 2 ;;
    --output-root) (($# >= 2)) || fail "missing value for $1"; output_root=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ -n "$nublar" && "$nublar" = /* && -f "$nublar" && -x "$nublar" && ! -L "$nublar" ]] || fail "--bin must name an absolute executable regular file"
[[ -n "$ecosystem_proof" && "$ecosystem_proof" = /* && -d "$ecosystem_proof" && ! -L "$ecosystem_proof" ]] || fail "--ecosystem-proof must name an absolute evidence directory"
nublar=$(cd "$(dirname "$nublar")" && pwd -P)/$(basename "$nublar")
ecosystem_proof=$(cd "$ecosystem_proof" && pwd -P)
repo_root=$(cd "$(dirname "$0")/.." && pwd -P)

args=(--bin "$nublar" --ecosystem-proof "$ecosystem_proof" --repo-root "$repo_root")
if [[ -n "$output_root" ]]; then args+=(--output-root "$output_root"); fi
exec python3 "$repo_root/acceptance/github-checks/mock.py" "${args[@]}"
