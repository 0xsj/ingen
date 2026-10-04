#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: acceptance/archive-cleanroom.sh \
  --image-id sha256:<64 lowercase hex> \
  --helper ABSOLUTE_PATH --helper-sha256 HEX \
  --archive ABSOLUTE_PATH --archive-sha256 HEX \
  --manifest-sha256 HEX --target GOOS/GOARCH

Uses an already-present immutable container image. It never pulls an image.
The run has no network, no capabilities, a read-only root filesystem, and a
non-root user. It verifies and installs archive bytes only; it never executes
the packaged native tools.
USAGE
}

fail() { printf 'archive-cleanroom: %s\n' "$1" >&2; exit 2; }

image_id=
helper=
helper_sha=
archive=
archive_sha=
manifest_sha=
target=
while (($#)); do
  case "$1" in
    --image-id) (($# >= 2)) || fail "missing value for $1"; image_id=$2; shift 2 ;;
    --helper) (($# >= 2)) || fail "missing value for $1"; helper=$2; shift 2 ;;
    --helper-sha256) (($# >= 2)) || fail "missing value for $1"; helper_sha=$2; shift 2 ;;
    --archive) (($# >= 2)) || fail "missing value for $1"; archive=$2; shift 2 ;;
    --archive-sha256) (($# >= 2)) || fail "missing value for $1"; archive_sha=$2; shift 2 ;;
    --manifest-sha256) (($# >= 2)) || fail "missing value for $1"; manifest_sha=$2; shift 2 ;;
    --target) (($# >= 2)) || fail "missing value for $1"; target=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "image ID must be an immutable sha256 ID"
for digest in "$helper_sha" "$archive_sha" "$manifest_sha"; do
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || fail "expected digests must be 64 lowercase SHA-256 hex characters"
done
[[ -n "$target" && "$target" != *..* && "$target" != /* && "$target" != *\\* ]] || fail "invalid target"
[[ -f "$helper" && ! -L "$helper" && "$helper" = /* ]] || fail "helper must be an absolute regular file"
[[ -f "$archive" && ! -L "$archive" && "$archive" = /* ]] || fail "archive must be an absolute regular file"
[[ "$helper$archive" != *$'\n'* && "$helper$archive" != *,* ]] || fail "mounted paths cannot contain comma or newline"
helper=$(cd "$(dirname "$helper")" && pwd -P)/$(basename "$helper")
archive=$(cd "$(dirname "$archive")" && pwd -P)/$(basename "$archive")

command -v docker >/dev/null 2>&1 || fail "docker is unavailable"
actual_image_id=$(docker image inspect --format '{{.Id}}' "$image_id") || fail "selected image is not already present"
[[ "$actual_image_id" = "$image_id" ]] || fail "selected image resolved to a different ID"

script_dir=$(cd "$(dirname "$0")" && pwd -P)
runner="$script_dir/archive-cleanroom/run.py"
[[ -f "$runner" && ! -L "$runner" ]] || fail "cleanroom runner is missing or a symlink"

exec docker run --rm --pull=never \
  --network none --read-only --cap-drop ALL \
  --cpus 2 --memory 1g --pids-limit 64 \
  --security-opt no-new-privileges:true --user 65534:65534 \
  --tmpfs /work:rw,noexec,nosuid,nodev,size=512m,uid=65534,gid=65534,mode=700 \
  --env PATH=/usr/bin:/bin --env HOME=/nonexistent --env TMPDIR=/work \
  --mount "type=bind,src=$helper,dst=/input/ingen-package.py,readonly" \
  --mount "type=bind,src=$archive,dst=/input/toolchain.tar.gz,readonly" \
  --mount "type=bind,src=$runner,dst=/input/archive-cleanroom.py,readonly" \
  --entrypoint /usr/bin/python3 "$image_id" \
  -I -S /input/archive-cleanroom.py \
  --helper /input/ingen-package.py --helper-sha256 "$helper_sha" \
  --archive /input/toolchain.tar.gz --archive-sha256 "$archive_sha" \
  --manifest-sha256 "$manifest_sha" --target "$target" --work-parent /work \
  --container-hardening
