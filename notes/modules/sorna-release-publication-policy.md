# Sorna tagged release bundle policy

Date: 2026-09-17

## Decision

A tag named `sorna-v<version>` starts the Sorna release-bundle job in
`.github/workflows/sorna-release.yml`. The tag must contain at least three
dot-separated version components and may otherwise use the same safe
alphanumeric, dot, underscore, and hyphen characters accepted by
`sorna/release.sh`.

The job reuses `make sorna-release-artifacts` and
`make sorna-release-provenance`. It runs the complete Sorna release checkpoint
first, builds the four current macOS/Linux archives, verifies every declared
SHA-256 digest, and uploads only the verified archives plus `SHA256SUMS`,
`release-manifest.json`, and
`release-verification.json`. It also emits `release-provenance.json`, which
binds the exact manifest and verification bytes to the source ref, tag, commit,
workflow, run, runner, and build timestamp.

Tagged bundles are retained as GitHub Actions artifacts for 90 days. This is
deliberately a CI distribution boundary, not yet a public GitHub Release.
Cryptographic signing, provenance attestation, and permanent release retention
remain separate decisions until the trust model and signing identity are
specified. The current provenance record is signing-ready evidence, not a
signature.

## Why this boundary

The tag job makes the release process repeatable and reviewable without
silently turning a repository tag into an externally published promise. The
verifier proves that the uploaded archive set matches its manifest; it does not
prove source provenance, oracle independence, or binary authenticity.

## Verification

The local equivalent remains:

```sh
make sorna-release-artifacts \
  SORNA_VERSION=0.1.0 \
  SORNA_COMMIT="$(git rev-parse --short HEAD)" \
  SORNA_BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
make sorna-release-verify \
  SORNA_VERSION=0.1.0 \
  SORNA_RELEASE_DIR=.artifacts/sorna-release/0.1.0
```
