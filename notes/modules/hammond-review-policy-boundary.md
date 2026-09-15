# A governance state must name the policy that materializes it

Hammond must not hide its approval threshold inside a state transition. A
record can only be called `approved` relative to a review policy applied to the
active review cycle.

## Origin

The first executable lifecycle treated the first approval event as an
immediate transition to `approved`, while the specification already described
approval as satisfying an active review policy. That made the default rule
look like universal governance semantics.

## What

The domain exposes a policy-aware validation and append path. The v1 default
policy requires one approval from one distinct actor in the active cycle.
Callers can supply a higher minimum approval count; the record remains
`in_review` until that policy is satisfied.

## Why

Approval thresholds are governance choices, not properties of an artifact
digest. Making the policy an input keeps state materialization deterministic
and leaves room for organization-specific quorum and role rules without
changing the event history.

## Gotchas

- Approval counts use distinct actor identities; repeating one actor does not
  satisfy a quorum.
- Policy-aware callers must use the policy-aware append/validate methods.
- The file store and CLI intentionally use the explicit one-approval default
  until a policy configuration format exists.
- A policy threshold does not establish identity, authorization, or role
  membership.

## Used in

- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/lifecycle.go`
- `hammond/internal/governance/validate.go`
- `hammond/internal/governance/amendment.go`
- `hammond/internal/governance/lineage.go`

## Related

- [Hammond governance specification](../../../hammond/GOVERNANCE-SPEC.md)
- [A review decision must belong to the review cycle it closes](hammond-review-cycle-binding.md)
