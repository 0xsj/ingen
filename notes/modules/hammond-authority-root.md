# Root-key rotation needs a caller-owned bootstrap

Hammond can model and verify root-key rotation, but it cannot decide how the
first root key becomes trusted.

## Origin

The authority trust snapshot already supported active and revoked issuer keys
and could be signed by a separately configured root verifier. The missing seam
was a versioned root snapshot that could rotate that verifier set without
turning the artifact into a self-authenticating root of trust.

## What

`AuthorityRootBootstrap` validates the caller's pinned initial keys and uses
them to load a digest-bound root snapshot. A signed replacement may then be
checked with a predecessor root set; after loading, only entries marked
`active` are exposed to verify the next root or authority trust snapshot.
Entries marked `revoked` remain auditable in the artifact but cannot verify new
signatures.

`LoadAuthorityTrustStoreWithRootStore` connects the active root set to a
root-signed authority trust snapshot. Canonical signing sorts key entries by
key ID, so equivalent key ordering cannot change the signed meaning.

## Why

The chain is now explicit:

```text
caller-approved bootstrap
          │
          ▼
signed root snapshot ──► active root keys
                              │
                              ▼
                    signed authority trust snapshot
                              │
                              ▼
                       authority verifier
```

This gives Hammond a deterministic, fail-closed rotation primitive while
keeping operational trust decisions outside the governance record validator.

## Gotchas

- `AuthorityRootBootstrap` must be supplied from deployment configuration or
  another approved out-of-band channel. A local URI and SHA-256 prove which
  root bytes were loaded, not who approved the first root key.
- A predecessor must sign the replacement while its key is still trusted; a
  revoked root cannot authorize a later snapshot.
- Version monotonicity, quorum approval, secure delivery, and recovery from a
  compromised root remain caller or deployment policy.
- A root signature authenticates the root snapshot only; it does not
  authenticate an organization provider or its membership semantics.

## Used in

- `hammond/internal/governance/types.go`
- `hammond/internal/governance/validate.go`
- `hammond/internal/governance/authority_root.go`
- `hammond/internal/governance/authority_trust.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/spec/ingen.hammond-authority-root-v1.schema.json`

## Related

- [Policy requirements and actor authority should be separate artifacts](hammond-authority-artifact.md)
- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
