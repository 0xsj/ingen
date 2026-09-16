# A role quorum counts distinct authorized actors

Overall approval thresholds and role-specific thresholds are separate policy
dimensions, but both must be deterministic and actor-distinct.

## Origin

Hammond already supported an overall minimum approval count and a list of roles
that each needed one approval. That was insufficient for policies such as
“three total approvals, including two distinct security reviewers.”

## What

`ReviewPolicy.RoleApprovalThresholds` maps a role to the minimum number of
distinct authorized actors who must approve in the active review cycle. The
`role_approval_thresholds` policy field implies role coverage; `RequiredRoles`
remains the shorthand for a threshold of one. When both specify a role, the
higher threshold applies.

Hammond evaluates authorization at each approval event's timestamp, counts
distinct actors overall and per role, and materializes `approved` only when all
configured thresholds are satisfied.

## Why

The policy can express quorum shape without changing the event model or
assuming that role names establish identity. The authority verifier remains
responsible for deciding whether an actor may use a role at the historical
decision time.

## Gotchas

- Repeating the same actor and role never increases the role count.
- An actor can count once toward each role they are authorized to use; the
  policy does not impose cross-role independence unless a future policy model
  explicitly adds it.
- Thresholds do not prove identity, organization membership, or reviewer
  independence beyond the configured authority verifier.
- Role threshold validation rejects empty role names and non-positive counts.

## Used in

- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/codec.go`
- `hammond/internal/governance/governance_test.go`
- `hammond/spec/ingen.hammond-review-policy-v1.schema.json`

## Related

- [A governance state must name the policy that materializes it](hammond-review-policy-boundary.md)
- [A role field is a claim until Hammond can verify authority](hammond-role-authority-boundary.md)
