# A role field is a claim until Hammond can verify authority

Hammond can require that approvals cover named roles, but the role string on an
event is not proof that the actor is authorized to use that role.

## Origin

The first review policy counted distinct actor strings and ignored the event
role. That made a policy unable to express that, for example, both product and
security review were required.

## What

The policy artifact may require one or more approval roles in addition to a
minimum number of distinct actors. It may reference a separate, versioned
authority artifact that carries explicit actor-to-role grants. The evaluator
checks required-role coverage and rejects approval or rejection events whose
actor/role pair is not granted by the loaded authority snapshot.

## Why

Role coverage is useful governance structure, but policy-declared grants must
not be mistaken for proof of real-world authorization. Actor identity, role
membership, delegation, and revocation still need an authority source outside
the local policy artifact.

## Gotchas

- Required roles are matched exactly after trimming whitespace.
- Authority verification receives the event timestamp, allowing future
  providers to apply effective-date and revocation rules.
- The authority reference and its bytes are verified before grants are used.
- The current evaluator verifies grants against the loaded local authority
  snapshot; it does not verify them against an identity provider.
- A single actor can hold multiple roles when the authority snapshot grants
  them.
- Policy identity and contract identity remain separate.

## Used in

- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/codec.go`
- `hammond/spec/ingen.hammond-authority-v1.schema.json`
- `hammond/spec/ingen.hammond-review-policy-v1.schema.json`

## Related

- [A policy digest is not verified until the policy bytes are loaded](hammond-policy-artifact-loading.md)
- [A policy-dependent governance state needs a policy identity](hammond-policy-identity.md)
