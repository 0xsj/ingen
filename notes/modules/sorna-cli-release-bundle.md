# Sorna CLI release bundle

Date: 2026-09-17

## Decision

Sorna now has a release-shaped bundle for the standalone CLI. The bundle
cross-compiles static binaries for macOS and Linux on amd64 and arm64, wraps
each binary in a versioned archive, records SHA-256 checksums, and emits a
`sorna.release/v1` manifest.

The output directory must be empty before a build. This prevents an old
archive from entering the checksum stream or manifest by accident.

## Verification boundary

`sorna release verify` loads the manifest with unknown-field and trailing-data
rejection, validates safe artifact names and lowercase SHA-256 digests, then
hashes every declared archive. A missing or changed archive produces a failed
`sorna.release-verification/v1` result and exit code 1; malformed manifest data
is an evaluation error.

The release bundle is packaging evidence, not an independent claim about
Sorna's isolation guarantees or the correctness of its contracts. Publication
and signing remain outside this slice.

## Verification

```sh
make sorna-release-artifacts \
  SORNA_VERSION=0.1.0 \
  SORNA_RELEASE_DIR=/private/tmp/ingen-sorna-release/0.1.0
make sorna-release-verify \
  SORNA_VERSION=0.1.0 \
  SORNA_RELEASE_DIR=/private/tmp/ingen-sorna-release/0.1.0
```
