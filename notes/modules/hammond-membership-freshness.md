# Freshness is a caller policy, not signature metadata

A signed membership response can be authentic and still be too old—or issued
from a clock that is too far in the future—to use for a new governance check.
Freshness must therefore be an explicit caller decision.

## Origin

`MembershipSnapshot` initially recorded `issued_at` but did not constrain how
long a caller could reuse the response. That left a gap between provenance and
operational freshness.

## What

`MembershipSnapshot.FreshAt` checks a caller-supplied maximum age, an allowed
future clock skew, and the optional snapshot `expires_at`. `VerifierAt` applies
those checks before returning a time-scoped membership verifier;
`VerifierAtWithProvenance` additionally binds that verifier to the snapshot
reference recorded on decision events.

## Why

Different consumers need different freshness windows. Keeping the clock and
limits at the caller boundary avoids hidden network behavior and prevents
Hammond from choosing an organization-specific staleness policy.

## Gotchas

- `issued_at` alone is metadata; it does not make a snapshot fresh.
- `expires_at` is an exclusive bound and must be after `issued_at`.
- Freshness checks need a caller-provided clock so tests and historical
  validation remain deterministic.
- Freshness does not prove completeness, transport security, or provider
  semantics.

## Used in

- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/spec/ingen.hammond-membership-v1.schema.json`

## Related

- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
- [Membership windows belong to the authority source](hammond-effective-membership.md)
