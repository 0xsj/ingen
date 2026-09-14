#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
cd "$repo_root"

version=${PADDOCK_VERSION:-dev}
commit=${PADDOCK_COMMIT:-unknown}
build_date=${PADDOCK_BUILD_DATE:-unknown}
release_dir=${PADDOCK_RELEASE_DIR:-"$repo_root/.artifacts/paddock-release/$version"}
targets=${PADDOCK_TARGETS:-"darwin/arm64 darwin/amd64 linux/amd64 linux/arm64"}
go_cache=${GOCACHE:-"$repo_root/.cache/paddock-go-build"}
go_mod_cache=${GOMODCACHE:-"$repo_root/.cache/paddock-go-mod"}

case "$version" in
	*[!A-Za-z0-9._-]*)
		echo "release version contains unsupported characters: $version" >&2
		exit 2
		;;
esac

mkdir -p "$release_dir"

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

	artifact="paddock_${version}_${os}_${arch}"
	GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
	GOCACHE="$go_cache" GOMODCACHE="$go_mod_cache" \
	go build -trimpath \
		-ldflags "-s -w -X ingen/paddock/internal/version.Version=$version -X ingen/paddock/internal/version.Commit=$commit -X ingen/paddock/internal/version.BuildDate=$build_date" \
		-o "$release_dir/$artifact" ./paddock/cmd/paddock
	tar -C "$release_dir" -czf "$release_dir/$artifact.tar.gz" "$artifact"
	digest_target="$artifact.tar.gz"
	printf 'BUILT %s/%s %s\n' "$os" "$arch" "$release_dir/$digest_target"
done

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$release_dir" && sha256sum paddock_*.tar.gz > SHA256SUMS)
else
	(cd "$release_dir" && shasum -a 256 paddock_*.tar.gz > SHA256SUMS)
fi

manifest="$release_dir/release-manifest.json"
{
	printf '{\n'
	printf '  "schema": "paddock.release/v1",\n'
	printf '  "name": "paddock",\n'
	printf '  "version": "%s",\n' "$version"
	printf '  "commit": "%s",\n' "$commit"
	printf '  "build_date": "%s",\n' "$build_date"
	printf '  "artifacts": [\n'
	first_artifact=yes
	while read -r digest filename; do
		if [ "$first_artifact" = yes ]; then
			first_artifact=no
		else
			printf ',\n'
		fi
		printf '    {"name": "%s", "sha256": "%s"}' "$filename" "$digest"
	done < "$release_dir/SHA256SUMS"
	printf '\n  ]\n'
	printf '}\n'
} > "$manifest"

printf 'CHECKSUMS %s\n' "$release_dir/SHA256SUMS"
printf 'MANIFEST %s\n' "$manifest"
