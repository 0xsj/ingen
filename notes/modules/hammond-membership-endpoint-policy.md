# Endpoint authorization must be explicit after URL validation

An absolute HTTP(S) URL is syntactically valid without being an approved
membership endpoint.

## Origin

The endpoint resolver added deployment-owned discovery, but URL parsing alone
did not express host allowlists, tenant restrictions, or an HTTPS-only rule.
The policy must run for both fixed and dynamically resolved endpoints.

## What

`HTTPMembershipProvider` supports `RequireHTTPS` and a caller-owned
`MembershipEndpointPolicy`. Hammond first validates the final URL, then applies
the HTTPS requirement and endpoint policy before invoking authentication or the
HTTP client. The same checks run on redirect targets before the client follows
them. URL userinfo and fragments are rejected by default.

`MembershipEndpointAllowlist` is a reusable exact host/port policy that can be
passed as `EndpointPolicy: allowlist.Validate`. It treats an omitted HTTPS
port as 443 and an omitted HTTP port as 80, and rejects wildcard hosts so the
deployment's approved destinations remain explicit.

## Why

This creates a clear preflight boundary: Hammond can enforce a deployment's
explicit endpoint decision without pretending to own DNS, TLS, redirects, or
network segmentation.

## Gotchas

- The policy applies to static and resolver-selected endpoints; a static URL
  must not bypass it.
- URL validation is not an allowlist. Callers should compare the final URL or
  host against their approved provider configuration, or use
  `MembershipEndpointAllowlist` for exact host/port matching.
- The allowlist is URL preflight only: it does not resolve DNS, validate TLS
  certificates, or enforce lower-level network tenancy. The caller's
  `http.Client` still controls redirect limits and whether to stop a redirect.
- `RequireHTTPS` does not validate certificates or pin a provider identity;
  those remain properties of the caller's HTTP client and trust configuration.
- Endpoint authorization does not replace request authentication or membership
  envelope signature verification.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Endpoint discovery is caller-owned and URL-validated](hammond-membership-endpoint.md)
- [Provider authentication is a caller-owned request capability](hammond-membership-authentication.md)
- [Transport can fetch membership bytes without owning provider identity](hammond-membership-transport.md)
