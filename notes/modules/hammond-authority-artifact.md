# Policy requirements and actor authority should be separate artifacts

A review policy says which roles are required; an authority artifact says which
actors are granted those roles. Keeping them separate lets policy change and
membership change be reviewed and rotated independently.

## Origin

Hammond first embedded actor-to-role grants directly in the policy artifact.
That was useful for a local prototype but coupled governance requirements to
membership data and made the authority boundary unclear.

## What

The policy may reference a versioned authority artifact. The loader verifies
the authority artifact's schema, identity, and exact SHA-256, then uses its
actor-to-role grants during policy evaluation.

The evaluator exposes this dependency through the `AuthorityVerifier`
interface. The verified local authority snapshot implements that interface;
remote or organization-backed implementations can be added at the runtime
boundary later.

For issuer attribution, an authority artifact may carry an Ed25519 signature
over its canonical `schema`, `id`, `version`, and `actors` payload. Hammond
accepts that signature only when the caller supplies a trusted key set through
`AuthoritySignatureVerifier`; the artifact's digest and signature answer
different questions.

The caller can represent its trust set as a versioned trust snapshot with
`active` and `revoked` Ed25519 keys. A new snapshot can retain a revoked old
key for audit while exposing only the replacement key to the verifier.

That trust snapshot may itself be signed by a root key. Hammond verifies the
root signature before deriving the active-key verifier, creating a local
root-to-authority chain without claiming to solve root-key distribution.

Normalized organization responses can use a separate signed membership
snapshot. Hammond verifies that response and adapts its effective-dated grants
to the same runtime verifier seam, without making a network request or
deciding provider-specific semantics.

## Why

Separating policy from authority makes the dependency visible: a decision is
evaluated against both the required-role rules and the membership snapshot
that granted an actor those roles.

## Gotchas

- A local unsigned authority artifact is still a declarative snapshot, not an
  identity provider.
- Policy and authority digests must both remain available for audit.
- A signature identifies a configured issuing key; key distribution, rotation,
  and revocation are enforced by the trust snapshot, but delivery and root
  approval for the root key are not solved by the artifact itself.
- A policy can be reused with a new authority reference only by publishing a
  new policy artifact.
- Delegation, revocation, and organization-backed authentication remain
  future work.

## Used in

- `hammond/internal/governance/types.go`
- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/codec.go`
- `hammond/internal/governance/authority_signature.go`
- `hammond/internal/governance/authority_trust.go`
- `hammond/internal/governance/membership.go`
- `hammond/spec/ingen.hammond-authority-v1.schema.json`
- `hammond/spec/ingen.hammond-authority-trust-v1.schema.json`
- `hammond/spec/ingen.hammond-membership-v1.schema.json`
- `hammond/spec/ingen.hammond-review-policy-v1.schema.json`

## Related

- [A role field is a claim until Hammond can verify authority](hammond-role-authority-boundary.md)
- [A policy digest is not verified until the policy bytes are loaded](hammond-policy-artifact-loading.md)
