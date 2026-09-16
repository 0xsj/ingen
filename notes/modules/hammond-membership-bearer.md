# A bearer token belongs to the caller's token source

Hammond can format a request credential without becoming the system that owns
or refreshes it.

## Origin

The provider authentication interface supported arbitrary request signing, but
simple bearer-token callers still had to repeat header formatting and token
validation at every integration site.

## What

`BearerTokenAuthenticator` receives a caller-owned
`MembershipBearerTokenSource`. For each request it asks the source for a token,
rejects empty or newline-containing values, and sets `Authorization: Bearer
<token>`. The authenticator retains the source, not the token.

## Why

The helper makes the common case consistent while preserving the important
ownership boundary: deployments decide how tokens are acquired, stored,
refreshed, rotated, scoped, and redacted.

## Gotchas

- A bearer header authenticates transport access only; the membership envelope
  still needs digest and issuer-signature verification.
- Token sources run once per request and must implement their own refresh and
  expiry behavior.
- Hammond does not retry a `401` or `403`, cache a token, or log credential
  values.
- Bearer tokens are only appropriate when the caller's HTTP client and endpoint
  policy provide the required transport protection.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Provider authentication is a caller-owned request capability](hammond-membership-authentication.md)
- [Endpoint authorization must be explicit after URL validation](hammond-membership-endpoint-policy.md)
- [Transport can fetch membership bytes without owning provider identity](hammond-membership-transport.md)
