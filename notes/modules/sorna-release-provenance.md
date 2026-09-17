# Sorna release provenance binds exact evidence bytes to build context

Date: 2026-09-17

## Decision

The Sorna release bundle now has a closed `sorna.release-provenance/v1`
envelope. It records the source repository, ref, tag, and commit; the workflow
run and runner context; the release build timestamp; and SHA-256 digests of the
exact `release-manifest.json` and `release-verification.json` files.

Creation requires a structurally valid manifest and a passed release
verification. The source commit and build timestamp must match the manifest,
and the tag must be `sorna-v<version>`. `sorna release provenance verify`
recomputes both file digests and rejects changed evidence or mismatched release
identity.

## Trust boundary

This record makes the release evidence reviewable and signing-ready. It does
not authenticate the source repository, prove that a runner was honest, prove
oracle independence, or make a CI artifact permanent. A future signing or
keyless attestation layer must bind this record to an independently trusted
identity rather than treating these self-reported fields as proof.

## Verification

The local workflow is:

```sh
make sorna-release-provenance \
  SORNA_VERSION=0.1.0 \
  SORNA_COMMIT="$(git rev-parse --short HEAD)" \
  SORNA_BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  SORNA_RELEASE_REPOSITORY="local" \
  SORNA_RELEASE_REF="local" \
  SORNA_RELEASE_TAG=sorna-v0.1.0
./.artifacts/sorna/sorna release provenance verify \
  --provenance .artifacts/sorna-release/0.1.0/release-provenance.json \
  --manifest .artifacts/sorna-release/0.1.0/release-manifest.json \
  --verification .artifacts/sorna-release/0.1.0/release-verification.json
```
