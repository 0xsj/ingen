# Authority must be evaluated at decision time

An actor's role membership is not necessarily timeless. A reviewer may be
granted a role, lose it, and later regain it; governance validation must give
an authority source the timestamp of the decision it is checking.

## Origin

Hammond's first authority verifier accepted only an actor and role. That was
enough for a static local snapshot, but it left no seam for effective dates,
revocation, or an organization-backed membership service.

## What

`AuthorityVerifier.Verify` receives the event's RFC3339 UTC timestamp. The
bundled `ReviewAuthority` intentionally ignores that value because its
artifact is a point-in-time snapshot. External implementations can evaluate
membership as of the decision timestamp and return an explicit error when the
membership source is unavailable.

The bundled `TimeScopedAuthority` adapter demonstrates inclusive start and
exclusive expiry windows for normalized membership data.

## Why

Passing the timestamp keeps historical validation deterministic and prevents a
future provider from silently answering a historical question with today's
membership. It also makes the authority decision's input set visible at the
governance boundary.

## Gotchas

- The event timestamp must be validated before it is treated as a trustworthy
  time input.
- A static authority snapshot does not gain temporal semantics merely because
  it receives a timestamp.
- Revocation policy—retroactive or effective from a future time—belongs to the
  authority provider, not to lifecycle transitions.
- Provider errors must fail validation; they must not be treated as a denial
  or approval.

## Used in

- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/validate.go`
- `hammond/internal/governance/governance_test.go`

## Related

- [Policy requirements and actor authority should be separate artifacts](hammond-authority-artifact.md)
- [A role field is a claim until Hammond can verify authority](hammond-role-authority-boundary.md)
