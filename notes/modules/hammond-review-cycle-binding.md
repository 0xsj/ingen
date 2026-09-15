# A review decision must belong to the review cycle it closes

Hammond must bind every approval or rejection to the specific review cycle that
opened it so a decision from an earlier attempt cannot approve a later one.

## Origin

The first lifecycle model recorded review-cycle IDs on `review-opened` events,
but decision events could omit them. That allowed a record with a rejected
cycle to carry an unscoped approval into a subsequent review.

## What

Each review opening has a unique cycle ID. Every approval or rejection carries
that same ID, and the validator checks it against the active cycle at that
point in the event history.

## Why

Rejection followed by re-review is a normal governance path. Without cycle
binding, an old approval remains indistinguishable from a new decision and can
silently satisfy the wrong review. Reviewer identity and artifact digest alone
do not identify the review attempt.

## Gotchas

- A new review cycle must use a new ID.
- The artifact digest still needs to match the governed contract.
- Cycle binding prevents stale decisions; it does not define quorum or role
  policy.
- A cycle ID is an event-level identity, not a replacement for the contract
  version or artifact digest.

## Used in

- `hammond/internal/governance/validate.go`
- `hammond/internal/governance/types.go`
- `hammond/spec/ingen.hammond-governance-v1.schema.json`

## Related

- [Hammond governance specification](../../../hammond/GOVERNANCE-SPEC.md)
- [A state label is not an executable transition](../concepts/a-state-label-is-not-a-transition.md)
