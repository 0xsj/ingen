#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
cd "$repo_root"

version=${SORNA_VERSION:-dev}
commit=${SORNA_COMMIT:-unknown}
build_date=${SORNA_BUILD_DATE:-unknown}
release_dir=${SORNA_RELEASE_DIR:-"$repo_root/.artifacts/sorna-release/$version"}
targets=${SORNA_RELEASE_TARGETS:-"darwin/arm64 darwin/amd64 linux/amd64 linux/arm64"}
go_cache=${GOCACHE:-"$repo_root/.cache/sorna-go-build"}
go_mod_cache=${GOMODCACHE:-"$repo_root/.cache/sorna-go-mod"}

case "$version" in
	*[!A-Za-z0-9._-]*)
		echo "release version contains unsupported characters: $version" >&2
		exit 2
		;;
esac

mkdir -p "$release_dir"
if test -n "$(find "$release_dir" -mindepth 1 -maxdepth 1 -print -quit)"; then
	echo "Sorna release directory is not empty: $release_dir" >&2
	exit 2
fi

for target in $targets; do
	os=${target%/*}
	arch=${target#*/}
	case "$os/$arch" in
		darwin/amd64|darwin/arm64|linux/amd64|linux/arm64) ;;
		*)
			echo "unsupported release target: $target" >&2
			exit 2
			;;
	esac

	artifact="sorna_${version}_${os}_${arch}"
	GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
	GOCACHE="$go_cache" GOMODCACHE="$go_mod_cache" \
	go build -trimpath \
		-ldflags "-s -w -X ingen/sorna/internal/version.Version=$version -X ingen/sorna/internal/version.Commit=$commit -X ingen/sorna/internal/version.BuildDate=$build_date" \
		-o "$release_dir/$artifact" ./sorna/cmd/sorna
	tar -C "$release_dir" -czf "$release_dir/$artifact.tar.gz" "$artifact"
	printf 'BUILT %s/%s %s\n' "$os" "$arch" "$release_dir/$artifact.tar.gz"
done

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$release_dir" && sha256sum "sorna_${version}_"*.tar.gz > SHA256SUMS)
else
	(cd "$release_dir" && shasum -a 256 "sorna_${version}_"*.tar.gz > SHA256SUMS)
fi

artifacts_json=$(jq -R -s '
	split("\n")
	| map(select(length > 0) | capture("^(?<sha>[0-9a-f]{64})  (?<name>.+)$") | {name: .name, sha256: .sha})
' "$release_dir/SHA256SUMS")
jq -n \
	--arg schema "sorna.release/v1" \
	--arg name "sorna" \
	--arg version "$version" \
	--arg commit "$commit" \
	--arg build_date "$build_date" \
	--argjson artifacts "$artifacts_json" \
	'{schema: $schema, name: $name, version: $version, commit: $commit, build_date: $build_date, artifacts: $artifacts}' \
	> "$release_dir/release-manifest.json"

printf 'CHECKSUMS %s\n' "$release_dir/SHA256SUMS"
printf 'MANIFEST %s\n' "$release_dir/release-manifest.json"
