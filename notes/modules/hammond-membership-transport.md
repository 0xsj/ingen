# Transport can fetch membership bytes without owning provider identity

An HTTP client is useful at the boundary, but it must not become an implicit
identity provider or trust decision-maker.

## Origin

Hammond already verified digest-bound, signed membership snapshots supplied as
local artifacts. The next missing seam was fetching the same envelope from a
live provider while keeping credentials and provider semantics caller-owned.

## What

`HTTPMembershipProvider` performs one bounded `GET` request to a static or
caller-resolved HTTP(S) endpoint. It accepts an injected `http.Client`,
invokes an optional `MembershipRequestAuthenticator` implementation, rejects
non-200 responses, and requires a positive response-size limit. `Fetch` expects the
provider to already return a Hammond envelope; `FetchNormalized` instead
hands raw bytes to a caller-owned normalizer before applying the same checks.

`FetchVerifierAt` additionally applies the caller's freshness policy before the
result can enter policy evaluation. `FetchVerifierAtWithProvenance` also binds
the returned verifier to the snapshot reference carried by decision events.

When `RequireHTTPS` or `EndpointPolicy` is configured, Hammond reapplies the
same endpoint checks to redirect targets before the injected client follows
them. A caller-provided `http.Client.CheckRedirect` remains responsible for
redirect limits and stop/continue behavior.
If normalization is requested, the source URI passed to the normalizer is the
final response request URL after those redirects, preserving the endpoint that
actually produced the bytes.

## Why

The adapter provides a reusable transport seam without guessing how a provider
issues credentials, discovers endpoints, maps organization records into
grants, or proves completeness. A deployment can supply those decisions
through its HTTP client, authentication hook, trust verifier, and normalization
process.

## Gotchas

- HTTP(S) transport does not authenticate the membership meaning; the signed
  snapshot and caller-owned verifier do that issuer check.
- Hammond does not store bearer tokens, refresh credentials, configure TLS, or
  define redirect and endpoint-discovery policy.
- The expected `MembershipReference` must be supplied by the caller, so the
  response remains bound to an identified artifact digest.
- A successful fetch is not proof that the provider returned a complete or
  semantically correct organization view.
- A normalizer may establish completeness for its provider, but that claim is
  not inferred from an HTTP status or from Hammond's signature check.
- The final response endpoint is source context, not an authorization proof;
  endpoint policy and TLS/network controls still belong to the caller.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/membership.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [An authenticated membership response is still a provider boundary](hammond-membership-provider.md)
- [Freshness is a caller policy, not signature metadata](hammond-membership-freshness.md)
- [Root-key rotation needs a caller-owned bootstrap](hammond-authority-root.md)
