# WORKING — Hammond's organization provider remains intentionally unselected

A provider-specific membership adapter cannot be implemented safely until its
credential protocol, endpoint ownership, and role-mapping semantics are known.

## Origin

Hammond now has bounded HTTP transport, caller-owned authentication, exact
endpoint policy, redirect checks, signed normalized snapshots, freshness rules,
and provenance binding. The remaining integration boundary is organization
specific, but no directory provider has been selected for Hammond.

## What

The current `HTTPMembershipProvider` is the stable provider-neutral seam. A
future adapter must supply the provider's credential implementation, endpoint
and TLS configuration, response normalization, completeness rules, issuer
trust configuration, and membership-to-role mapping. It should return the
existing signed `MembershipSnapshot` rather than introduce provider semantics
into governance validation.

## Why

Choosing a provider by inference would make credential scope, token refresh,
redirect behavior, pagination, group nesting, revocation, and role mapping
look generic when they are not. The losing alternative is a guessed adapter
that could authorize the wrong people or leak credentials while appearing to
be a reusable Hammond feature.

## Required choice

Before implementation, select the organization provider and confirm:

- the membership API and authentication protocol;
- the credential source, scope, refresh, and redaction rules;
- the approved endpoint, TLS, redirect, and deployment network policy;
- the provider's completeness and pagination semantics; and
- the mapping from provider identities/groups to Hammond actors and roles.

## Gotchas

- A bearer token proves transport access, not membership meaning.
- A signed normalized snapshot proves the bytes and issuer key, not that the
  normalizer mapped every provider member correctly.
- Provider-specific credentials must stay outside governance records and
  membership artifacts.
- The selected adapter must preserve the membership reference and source
  provenance used by the decision event.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/membership.go`
- `hammond/status.md`
- `hammond/GOVERNANCE-SPEC.md`

## Related

- [An authenticated membership response is still a provider boundary](../modules/hammond-membership-provider.md)
- [Provider authentication is a caller-owned request capability](../modules/hammond-membership-authentication.md)
- [Endpoint authorization must be explicit after URL validation](../modules/hammond-membership-endpoint-policy.md)
