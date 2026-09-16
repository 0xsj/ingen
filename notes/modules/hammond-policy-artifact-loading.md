# A policy digest is not verified until the policy bytes are loaded

Recording a policy reference makes the governance context identifiable, but
the reference alone does not prove that the policy bytes were read or that
their contents match the threshold used for evaluation.

## Origin

Hammond records a policy URI and SHA-256 and accepts a `ReviewPolicy` value from
the caller. Without a loader, a caller could pair the right-looking reference
with a different in-memory threshold.

## What

The governance package must decode policy artifacts strictly, verify the
policy schema and identity, hash the exact input bytes, and only then produce a
policy usable for evaluation.

## Why

The policy artifact is part of the approval decision. Digest verification
prevents a mutable path or accidental configuration drift from changing the
meaning of an existing governance record.

## Gotchas

- The URI locates the bytes; the digest is the policy identity.
- Strict decoding rejects unknown policy fields and trailing JSON.
- Loading a policy proves byte and shape agreement, not that the actor is
  authorized to publish or use it.
- The local loader accepts filesystem paths only; organization identity, role
  authority, and remote policy distribution remain outside it.

## Used in

- `hammond/internal/governance/policy.go`
- `hammond/internal/governance/codec.go`

## Related

- [A policy-dependent governance state needs a policy identity](hammond-policy-identity.md)
- [A governance schema must be enforced at the load boundary](hammond-schema-runtime-validation.md)
