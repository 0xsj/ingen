# A provenance-carrying verifier must match the decision reference

Recording a source reference is useful only when the runtime verifier can be
bound to that same source.

## Origin

Decision events gained an optional membership reference for audit provenance,
but an opaque `AuthorityVerifier` interface could not prove which snapshot had
produced it. That left a caller able to record one reference while injecting a
different verifier.

## What

`MembershipVerifier` wraps a time-scoped authority with its
`MembershipReference`. `VerifierAtWithProvenance` and
`HTTPMembershipProvider.FetchVerifierAtWithProvenance` create this wrapper
after freshness checks. During policy evaluation, Hammond requires approval
and rejection events to carry an equal membership reference when this wrapper
is injected.

## Why

This closes the common provenance mismatch without requiring Hammond to
understand every custom authority implementation. Existing opaque verifiers
remain usable, but their caller must preserve provenance outside this binding
mechanism.

## Gotchas

- The event reference is compared to the wrapper's reference; Hammond still
  does not reload the artifact during record validation.
- Freshness is checked when the provenance-carrying verifier is created, while
  grant effective windows are checked at each historical decision timestamp.
- A custom `AuthorityVerifier` can opt out of automatic reference matching and
  must provide its own provenance contract if one is required.
- The binding proves source identity, not provider completeness or the
  business correctness of normalization.

## Used in

- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Decision provenance should name the membership snapshot](hammond-membership-provenance.md)
- [Freshness is a caller policy, not signature metadata](hammond-membership-freshness.md)
- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
