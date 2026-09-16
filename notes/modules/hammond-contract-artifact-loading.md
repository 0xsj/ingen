# A contract reference is only integrity-checked when its bytes are loaded

Hammond may verify that the bytes located by a contract reference match the
recorded SHA-256, but it must not become a second contract parser or
canonicalizer.

## Origin

Hammond bound decisions to the digest stored in a contract reference and in
approval events, but did not yet provide a byte-level loader for the referenced
artifact.

## What

The governance package exposes a local contract-artifact loader that reads the
referenced bytes and compares their SHA-256 with the reference. It performs no
Sorna semantic validation and returns the original bytes unchanged.

## Why

A mutable path can otherwise continue to point at different bytes while the
governance record and its approval history still look valid. The digest check
protects artifact integrity while preserving Sorna's ownership of contract
meaning.

## Gotchas

- The loader verifies bytes, not contract syntax, sealing, behavior, or
  evidence.
- The local loader accepts a filesystem path, not an `http://`, `https://`, or
  `file://` URI; remote retrieval is not implied.
- Membership HTTP transport is a separate explicit boundary and does not make
  contract, policy, or authority loading network-capable.
- Callers must use the loader at their ingress boundary; pure domain
  validation intentionally remains backend-independent.

## Used in

- `hammond/internal/governance/codec.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/internal/store/filesystem.go`

## Related

- [Hammond approval governs identified bytes, not behavior](hammond-governance-artifact-identity.md)
- [A policy digest is not verified until the policy bytes are loaded](hammond-policy-artifact-loading.md)
