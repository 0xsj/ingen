# DEFERRED — GitHub team membership is an optional future provider

Hammond's current active workflow is solo and local. It uses a local authority
artifact, so no GitHub organization or external membership provider is needed.
The GitHub team adapter is retained as an isolated future option for the point
when Hammond needs shared reviewers across projects.

## Origin

Hammond already has bounded HTTP transport, caller-owned authentication, exact
endpoint policy, redirect checks, signed normalized snapshots, freshness rules,
and provenance binding. Those seams are prepared, but the solo workflow does
not activate them.

## What

The future GitHub adapter uses `GET /orgs/{org}/teams/{team_slug}/members`
with `per_page=100`, explicit page numbers, and `role=all`. GitHub may include
child-team members; the adapter ignores entries marked `inherited=true` and
maps direct members to one caller-selected Hammond role. It uses the stable
numeric GitHub user ID as `github:user:<id>`.

If activated later, the caller supplies the API base URL, token authenticator,
issuer signer, issuer trust set, snapshot identity/version, and freshness
metadata. Hammond does not read `gh` credentials, store tokens, manage
refresh, or infer roles.

The adapter returns the existing signed `MembershipSnapshot` rather than
introducing GitHub semantics into governance validation.

The future-provider acceptance worksheet is kept in
[`hammond/PROVIDER-INTEGRATION-CHECKLIST.md`](../../hammond/PROVIDER-INTEGRATION-CHECKLIST.md).
The implementation is covered by deterministic HTTP fixtures; no live GitHub
credential is required by the test suite.

## Why

GitHub is a useful future option because the repository already uses GitHub
Actions as a concrete integration environment. It is not the current source
of authority: for a solo operator, a local authority artifact is simpler,
deterministic, and avoids making an organization boundary part of the active
workflow.

## Required choice

If a shared organization workflow is activated later, its boundary is:

- membership API: GitHub REST team-members endpoint;
- authentication: caller-owned bearer-token authenticator, using the minimum
  GitHub organization Members read permission;
- endpoint: HTTPS GitHub API base URL, exact caller allowlist, and redirect
  validation;
- completeness: page until a response contains fewer than `per_page` members,
  with a configurable maximum page count; exhausting the cap fails closed;
- membership semantics: direct members only; inherited child-team members are
  excluded; duplicate user IDs fail closed;
- actor mapping: stable numeric user ID to `github:user:<id>`;
- role mapping: every direct member receives the configured Hammond role; and
- issuer trust: caller-owned Ed25519 signer and verifier for the normalized
  snapshot.

## Gotchas

- A bearer token proves transport access, not membership meaning.
- A signed normalized snapshot proves the bytes and issuer key, not that the
  normalizer mapped every provider member correctly.
- Provider-specific credentials must stay outside governance records and
  membership artifacts.
- The selected adapter must preserve the membership reference and source
  provenance used by the decision event.
- GitHub's `role` field describes the member's GitHub team role; it is not
  copied into Hammond's role vocabulary.
- `gh login` is a local operator convenience and is not an adapter credential
  contract.

## Used in

- `hammond/internal/governance/membership_provider.go`
- `hammond/internal/governance/membership.go`
- `hammond/status.md`
- `hammond/GOVERNANCE-SPEC.md`

## Official provider reference

- [GitHub REST team-members endpoints](https://docs.github.com/en/rest/teams/members)
- [GitHub permissions required for fine-grained tokens](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens)

## Related

- [An authenticated membership response is still a provider boundary](../modules/hammond-membership-provider.md)
- [Provider authentication is a caller-owned request capability](../modules/hammond-membership-authentication.md)
- [Endpoint authorization must be explicit after URL validation](../modules/hammond-membership-endpoint-policy.md)
