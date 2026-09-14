# Paddock release checklist

Paddock releases are static command-line artifacts. The release path is
provider-neutral so it can run from any CI system with Go and a POSIX shell.

## Verify

Run the release-shaped checks first:

```sh
make -C paddock release-check \
  VERSION=0.1.0 \
  COMMIT="$(git rev-parse --short HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

This builds the CLI, runs the full Paddock test suite, runs `go vet`, and
checks text and JSON version output.

## Build artifacts

Build the supported first-release targets:

```sh
make -C paddock release-artifacts \
  VERSION=0.1.0 \
  COMMIT="$(git rev-parse --short HEAD)" \
  BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
```

The output directory contains archives for macOS and Linux on amd64 and arm64,
`SHA256SUMS`, and a `release-manifest.json` using the
`paddock.release/v1` contract in [`spec/`](spec/). Builds use `CGO_ENABLED=0`
and `-trimpath`; version metadata is embedded in every binary.

The manifest is the publishing handoff: a CI provider can upload the archives,
checksum file, and manifest together without interpreting build logs.

Verify the handoff before publishing:

```sh
make -C paddock release-verify \
  RELEASE_MANIFEST=/path/to/release-manifest.json
```

The verifier checks every declared archive in the manifest directory and emits
`paddock.release-verification/v1` in JSON mode. A missing or changed archive is
a failed verification; malformed manifest data is an evaluation error.

Before publishing, verify that:

- the policy and graph protocol schemas have not changed without a versioning decision;
- the Paddock acceptance suite and `go vet` pass;
- the generated checksums match the uploaded archives;
- a downloaded binary reports the intended version, commit, and build date;
- the locked Overwatch Go and TypeScript dogfood gates still produce their documented results.
