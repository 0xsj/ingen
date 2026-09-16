# Handling history belongs in append-only events

Lockwood's custody record contains the metadata known at intake. Later
redaction observations, retention classifications, and legal-hold decisions
must not silently rewrite that record or the artifact it names. They belong in
a separate, immutable handling-event stream.

## Origin

The first custody record already had `handling.redaction` and
`handling.retention_class`, but those fields could not represent later
decisions without changing historical bytes. The handling-event slice adds a
versioned event contract and durable storage keyed by custody ID and event ID.

## What

`lockwood.handling-event/v1` supports `redaction`, `retention-classified`,
`legal-hold-placed`, and `legal-hold-released` events. Each event has an
explicit time, descriptive actor and reason, and type-specific references.
Filesystem publication is atomic and durable; repeated identical event IDs
are idempotent, while conflicting contents are rejected. The CLI exposes
`append-event` and `list-events`.

## Why

Separate events preserve the custody record's canonical representation and
make the handling timeline inspectable. This keeps Lockwood honest about what
it has implemented: recording a decision is not the same as authenticating
the actor, deleting bytes, redacting a payload, or enforcing a policy.

## Gotchas

- The `actor` field is a descriptive caller claim, not an authenticated
  identity or authorization decision.
- A redaction event names original and resulting digests, but the current
  command does not create the resulting artifact or perform redaction.
- Retention and legal-hold events are records only; deletion, access control,
  and hold enforcement need a future governance and storage boundary.
- Events do not participate in custody lineage and do not change record
  status.

## Used in

- [`lockwood/internal/custody`](../../lockwood/internal/custody/)
- [`lockwood append-event`](../../lockwood/cmd/lockwood/)
- [`Lockwood custody specification`](../../lockwood/CUSTODY-SPEC.md)

## Related

- [`A state label is not an executable transition`](../concepts/a-state-label-is-not-a-transition.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
