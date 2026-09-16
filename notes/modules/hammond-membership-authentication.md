# Provider authentication is a caller-owned request capability

Transport authentication should be extensible without making Hammond a
credential manager.

## Origin

The HTTP membership adapter initially accepted a function that could attach a
header to a request. That handled a bearer token, but did not give callers a
clear seam for stateful token refresh or request-signing implementations.

## What

`MembershipRequestAuthenticator` is an interface invoked once after endpoint
resolution and before the HTTP request is sent. `MembershipRequestAuthenticatorFunc`
adapts simple functions; stateful implementations can refresh or sign through
their own dependencies. Hammond never serializes the authenticator into a
membership artifact or interprets its credentials.

`BearerTokenAuthenticator` is a small generic implementation backed by a
caller-owned `MembershipBearerTokenSource`. It resolves one token from the
request context, rejects empty or newline-containing values, and sets the
`Authorization` header for that request.

The membership issuer signature remains a separate check: request
authentication controls transport access, while the configured signature
verifier authenticates the returned envelope's issuer.

## Why

Keeping request credentials at the caller boundary supports bearer tokens,
mTLS-backed clients, signed requests, and organization-specific mechanisms
without selecting one provider's identity protocol for Hammond.

## Gotchas

- The authenticator runs once per request; Hammond does not refresh credentials
  or retry after an authentication failure.
- A non-200 response, including `401` or `403`, is a transport failure and does
  not prove that membership is absent or invalid.
- Callers own secret storage, rotation, redaction, TLS configuration, and
  credential scope.
- The bearer helper does not refresh or cache tokens; its source must provide
  the right token for each request.
- A successful request does not replace digest, issuer-signature, freshness, or
  completeness checks.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Transport can fetch membership bytes without owning provider identity](hammond-membership-transport.md)
- [Endpoint discovery is caller-owned and URL-validated](hammond-membership-endpoint.md)
- [Root-key rotation needs a caller-owned bootstrap](hammond-authority-root.md)
