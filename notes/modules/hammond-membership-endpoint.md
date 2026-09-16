# Endpoint discovery is caller-owned and URL-validated

Provider discovery can vary by organization and environment, so Hammond should
accept the result without becoming a service registry.

## Origin

The membership transport initially accepted only a fixed endpoint. That was
enough for a local test server but forced callers to wrap Hammond for ordinary
service discovery.

## What

`MembershipEndpointResolver` lets the caller return an endpoint at fetch time.
`HTTPMembershipProvider` accepts either a fixed `Endpoint` or a resolver, never
both, and validates the selected URL as an absolute `http` or `https` endpoint
with a host before making the request. The resolved endpoint is passed to the
normalizer as source context. A separate endpoint policy can reject the final
URL before the request is sent.

## Why

The seam supports deployment-owned service discovery while keeping Hammond's
transport contract small. Hammond validates the shape of the result, but does
not decide which registry, DNS system, region, tenant, or failover policy the
caller should use.

## Gotchas

- Endpoint URL validation is not endpoint authorization; callers still own
  allowlists, TLS configuration, redirects, and network access policy. Use the
  endpoint policy hook when those decisions must block the request in Hammond.
- A resolver may return different endpoints across calls, so callers should
  preserve the resolved endpoint with the fetched artifact provenance.
- Authentication runs after endpoint resolution and can inspect the final
  request URL.
- Discovery failures and non-200 responses are transport errors, not evidence
  that a membership view is empty or complete.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Transport can fetch membership bytes without owning provider identity](hammond-membership-transport.md)
- [Normalization can reject an incomplete provider view](hammond-membership-normalization.md)
