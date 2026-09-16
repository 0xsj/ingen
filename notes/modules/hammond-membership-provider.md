# An authenticated membership response is still a provider boundary

Hammond can verify the shape, bytes, issuer signature, and effective windows of
a normalized membership response. It cannot decide whether an organization
provider's response is fresh, complete, or semantically correct.

## Origin

The time-scoped authority adapter made historical role checks possible, but it
accepted caller-supplied grants without provenance. A signed membership
snapshot adds an auditable provider response without making Hammond a network
client.

## What

`MembershipSnapshot` carries a versioned reference, `issued_at`, optional
`expires_at`, normalized effective-dated grants, and an optional Ed25519
signature. The loader verifies the exact artifact digest and, when given a
trusted provider key set, the canonical response signature. Its `VerifierAt`
method lets callers enforce freshness before returning the same timestamp-aware
membership interface used by lifecycle validation.

## Why

Separating response verification from provider semantics keeps Hammond's
boundary honest. The caller chooses the provider, trust set, freshness window,
and any completeness rules before injecting the resulting verifier.

## Gotchas

- `issued_at` records provider metadata; callers must use `FreshAt` or
  `VerifierAt` to enforce freshness.
- A valid signature authenticates the configured issuer key, not the provider's
  internal directory semantics.
- The membership reference is not yet embedded in the v1 governance record;
  callers that inject it must preserve that provenance alongside the record.
- Network transport, credentials, and provider-specific mapping remain outside
  Hammond; callers still choose the freshness limits.

## Used in

- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/authority_membership.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/spec/ingen.hammond-membership-v1.schema.json`

## Related

- [Authority must be evaluated at decision time](hammond-authority-time.md)
- [Membership windows belong to the authority source](hammond-effective-membership.md)
- [Policy requirements and actor authority should be separate artifacts](hammond-authority-artifact.md)
