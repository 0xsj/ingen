# GitHub team membership is an optional future provider adapter

Hammond's optional organization provider is GitHub team membership. The
adapter is intentionally narrower than a general GitHub organization model:
one configured team supplies direct members for one configured Hammond role.
The current solo workflow uses a local authority artifact instead.

## What

`GitHubTeamMembershipProvider` reads
`GET /orgs/{org}/teams/{team_slug}/members` with explicit page numbers and up
to 100 entries per page. It continues until a short page, rejects duplicate
or structurally invalid members, and fails closed when a configured page cap is
exhausted. GitHub entries with `inherited=true` are excluded so child-team
membership cannot silently expand the first role mapping.

Direct members are represented as `github:user:<numeric-id>`. The GitHub team
role is deliberately not copied into Hammond's role vocabulary; every direct
member receives the configured Hammond role.

The caller supplies the bearer-token authenticator, API endpoint policy,
snapshot ID/version and timestamps, Ed25519 issuer signer, and issuer trust
verifier. The adapter returns a signed, digest-bound `MembershipSnapshot` and
does not read `gh` configuration or store credentials.

## Why

GitHub is already a concrete integration environment in this repository, so it
provides a useful future vertical slice without inventing a universal directory
vocabulary. Keeping the role mapping explicit prevents GitHub's `member` and
`maintainer` labels from being mistaken for Hammond policy roles. It is not
needed while Hammond has one local operator.

## Gotchas

- GitHub may include child-team members in the team response; inherited
  entries are intentionally ignored in this first slice.
- Numeric GitHub user IDs are used instead of mutable login names. Consumers
  must record approval actors using the mapped `github:user:<id>` value.
- A successful GitHub response is not itself a Hammond authority assertion;
  the caller-owned issuer signs the normalized snapshot and the caller-owned
  trust set verifies it.
- The local membership-version ledger still needs to accept the verified
  snapshot before it becomes the durable current version.

## Used in

- `hammond/internal/governance/github_membership.go`
- `hammond/internal/governance/github_membership_test.go`
- `notes/decision-adjacent/hammond-provider-integration-boundary.md`
- [GitHub REST team-members endpoints](https://docs.github.com/en/rest/teams/members)
