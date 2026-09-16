# A policy-dependent governance state needs a policy identity

If Hammond materializes `approved` using a review policy, the governance
record must identify the exact policy artifact used. A threshold supplied only
as an in-memory option is not enough to reproduce or audit the decision.

## Origin

The first policy-aware lifecycle accepted a minimum approval count, but records
stored no policy reference. The same event history could therefore be loaded
under a different threshold and receive a different meaning.

## What

Every v1 record carries a policy reference containing policy ID, version,
schema, URI, and SHA-256. Policy-aware validation requires the supplied policy
to match that reference. The local CLI and store use the versioned one-actor
policy artifact.

## Why

Policy bytes are part of the governance context. Binding them by digest keeps
the state projection deterministic while leaving policy content separate from
the contract artifact and event history.

## Gotchas

- A policy reference identifies bytes; it does not prove that those bytes were
  loaded or authorized by an organization.
- Policy identity is separate from contract identity.
- Role authority, identity providers, and policy distribution remain future
  boundaries.

## Used in

- `hammond/internal/governance/types.go`
- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/validate.go`
- `hammond/spec/ingen.hammond-governance-v1.schema.json`

## Related

- [A governance state must name the policy that materializes it](hammond-review-policy-boundary.md)
- [Hammond approval governs identified bytes, not behavior](hammond-governance-artifact-identity.md)
