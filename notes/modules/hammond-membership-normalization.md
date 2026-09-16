# Normalization can reject an incomplete provider view

Provider-native membership data needs an explicit mapping boundary before it
can participate in Hammond policy evaluation.

## Origin

The HTTP adapter can fetch bytes, but providers do not naturally return
Hammond's versioned actor-role envelope. Treating arbitrary provider JSON as if
it were already normalized would make Hammond guess field meanings and
completeness.

## What

`MembershipResponseNormalizer` is a caller-owned function that receives the
provider-native response and source URI, then returns a
`NormalizedMembershipResponse` containing Hammond envelope bytes and their
`MembershipReference`. `HTTPMembershipProvider.FetchNormalized` passes that
result through strict decoding, exact digest verification, and the configured
issuer signature verifier.

The normalizer may reject a response that is incomplete for its provider or
scope. Hammond does not infer completeness from HTTP status, field presence, or
signature validity.

## Why

This keeps provider semantics at the integration edge. Each deployment can
define how groups, teams, scopes, removals, and pagination map into effective
grants without changing Hammond's lifecycle or policy validator.

## Gotchas

- The reference digest identifies the normalized envelope returned by the
  normalizer; raw-provider provenance needs to be retained by the caller.
- A valid issuer signature authenticates the normalized bytes, not the
  normalizer's business interpretation or completeness claim.
- Normalization is not authorization: policy evaluation still needs the
  resulting authority verifier and decision-time grant checks.
- Provider-specific pagination, retries, endpoint discovery, and credentials
  remain outside this boundary.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Transport can fetch membership bytes without owning provider identity](hammond-membership-transport.md)
- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
- [Freshness is a caller policy, not signature metadata](hammond-membership-freshness.md)
