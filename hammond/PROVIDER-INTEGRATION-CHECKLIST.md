# Hammond provider-integration checklist

Status: GitHub team membership deferred; solo local authority is active

This is a preparation worksheet, not a provider API contract. It defines the
acceptance boundary that any selected organization adapter must satisfy while
leaving credentials, endpoint details, pagination, and role semantics to the
selected provider and its deployment.

## Future GitHub slice

| Field | Decision |
| --- | --- |
| Provider | GitHub REST `GET /orgs/{org}/teams/{team_slug}/members` |
| Consumer | Hammond contract-approval policy evaluation |
| Authentication | Caller-owned bearer token; minimum organization Members read permission |
| Endpoint | HTTPS GitHub API base URL, exact host/port policy, redirect validation |
| Completeness | Explicit pagination through a short page; maximum page cap fails closed |
| Mapping | Direct members only; `github:user:<numeric-id>` to one configured Hammond role |
| Issuer trust | Caller-owned Ed25519 signer and verifier for normalized snapshots |
| Freshness | Caller supplies `issued_at`/`expires_at` and applies the existing freshness policy |
| Persistence | Existing local membership version ledger remains sufficient for v1 |

## Existing Hammond handoff

The adapter must use the existing provider-neutral seams:

1. Caller-owned credential and endpoint configuration.
2. `HTTPMembershipProvider` for bounded HTTP transport and redirect checks,
   when HTTP is appropriate.
3. `MembershipResponseNormalizer` for provider-specific mapping and
   completeness decisions.
4. A signed, digest-bound `MembershipSnapshot` as the normalized output.
5. Freshness and monotonic-version checks before authorization.
6. `MembershipVerifier` or `AuthorityVerifier` at the decision timestamp.
7. A matching membership reference on approval/rejection events.

The adapter must not add provider semantics to governance validation or turn a
bearer token into an authority claim.

## Selection record

Complete these fields before implementation:

| Field | Required decision |
| --- | --- |
| Provider | Organization/directory system and API version |
| Consumer | Workflow or service that will use the membership result |
| Authentication | Credential source, scope, refresh, rotation, and redaction |
| Endpoint | Approved URL/discovery path, TLS, redirects, and network policy |
| Completeness | Pagination, partial responses, deletions, and revocation behavior |
| Mapping | Provider identities/groups to Hammond actors and roles |
| Issuer trust | Signing key delivery, rotation, and revocation |
| Freshness | Maximum age, future skew, and expiry policy |
| Persistence | Whether the local version ledger is sufficient for the consumer |

## Acceptance cases

The selected adapter is not ready until it demonstrates:

- valid credentials are requested through the caller-owned source and are not
  written into Hammond artifacts or logs;
- disallowed schemes, hosts, ports, redirects, or TLS conditions are rejected
  before provider data enters policy evaluation;
- incomplete, paginated, revoked, and deletion-bearing provider views follow
  the provider-specific completeness decision;
- the normalizer emits a valid Hammond membership envelope with exact digest,
  identity, grant windows, and issuer signature;
- unknown, revoked, malformed, or invalid issuer signatures fail closed;
- stale, expired, future-dated, lower-version, and same-version conflicting
  snapshots are rejected;
- a valid snapshot authorizes only the mapped actor/role pairs at the event's
  UTC timestamp;
- the approval/rejection event carries the exact membership reference used by
  the verifier; and
- a repeatable test covers the full provider response through policy
  authorization and provenance validation.

## Required deliverables

Before merging a provider-specific slice, add:

- a provider decision note naming the selected consumer and boundaries;
- redacted request/response fixtures or an equivalent deterministic test
  server;
- the provider normalizer and its completeness tests;
- credential, endpoint, issuer-trust, freshness, and versioning tests;
- one end-to-end Hammond policy/provenance test; and
- an update to `roadmap.md`, `status.md`, and the relevant learning note.

## Explicit non-goals

This checklist does not authorize or define:

- a particular organization provider;
- hosted Hammond APIs or remote artifact retention;
- deployment-wide TLS or network enforcement;
- secure root bootstrap delivery;
- automatic approval from Sorna results; or
- a universal organization-to-Hammond role vocabulary.
