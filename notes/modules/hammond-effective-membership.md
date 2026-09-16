# Membership windows belong to the authority source

Role membership can change over time. Hammond should ask whether an actor had
a role at the decision timestamp, while the authority source owns the rules
for effective dates, expiry, and revocation.

## Origin

The local authority snapshot began as a static actor-to-role map. Hammond then
made the verifier timestamp-aware so an organization-backed provider would not
have to answer historical decisions using only today's membership.

## What

`TimeScopedAuthority` adapts normalized, effective-dated grants to
`AuthorityVerifier`. A grant starts at inclusive `valid_from` and ends at
exclusive `valid_until`; an empty end has no recorded expiry. Invalid UTC
windows and provider errors fail closed.

## Why

The timestamp is part of the authority question, not merely event metadata.
Keeping temporal semantics in the authority adapter avoids putting revocation
policy into Hammond's lifecycle transitions and preserves deterministic
historical validation.

## Gotchas

- The adapter accepts normalized grants; it does not authenticate their source.
- A static signed snapshot remains static even when the verifier receives a
  timestamp.
- Exclusive expiry avoids ambiguity at the exact revocation instant.
- Retroactive revocation is a provider policy decision and must be documented
  by the provider.

## Used in

- `hammond/internal/governance/authority_membership.go`
- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Authority must be evaluated at decision time](hammond-authority-time.md)
- [Policy requirements and actor authority should be separate artifacts](hammond-authority-artifact.md)
