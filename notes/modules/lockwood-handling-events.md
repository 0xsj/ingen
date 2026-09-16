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
`append-event` and `list-events`. A read-only `handling-status` projection
derives the visible retention class, redaction history, and active holds from
the event stream without implying enforcement. Events can also carry a
detached Ed25519 signature envelope bound to the exact event digest; the
existing trust registry can verify the signing key.
The `lockwood.handling-event-policy/v1` snapshot then separately allowlists
which trusted key IDs may sign each event type; authorized verification
reports both snapshot digests.

## Why

Separate events preserve the custody record's canonical representation and
make the handling timeline inspectable. This keeps Lockwood honest about what
it has implemented: recording a decision is not the same as authenticating
the actor, deleting bytes, redacting a payload, or enforcing a policy.

## Gotchas

- The `actor` field is a descriptive caller claim, not an authenticated
  identity or authorization decision.
- A redaction event names original and resulting digests. `register-redaction`
  now verifies a caller-produced resulting artifact and appends the event, but
  it does not create that artifact, transform payloads, or perform deletion.
- Retention and legal-hold events are records only; deletion, access control,
  and hold enforcement need a future governance and storage boundary.
- Events do not participate in custody lineage and do not change record
  status.
- A projected active hold only means a placement event has no later release in
  the visible stream; it is not an access-control decision.
- A trusted handling-event signature authenticates control of a key, not the
  human named by `actor` or the authorization and execution of the action.
- Signed event envelopes are separate artifacts and do not make the unsigned
  event stream authenticated by default.
- The action policy is an explicit key/type allowlist, not a human directory
  or proof that the authorized action was executed.
- The first enforcement boundary is deliberately read-only: a handling guard
  blocks `redact` and `delete` when an active legal hold is visible. A clear
  result is only “not blocked by a visible hold”; it is not permission and does
  not evaluate retention age, actor identity, or storage mutation capability.
- `register-redaction` preserves the original blob and custody record and does
  not automatically create a new custody record for the result. The built-in
  memory and filesystem stores serialize handling-event mutations per custody
  ID, so concurrent registrations cannot both branch from the same current
  source. On flock-capable platforms, filesystem locking is local advisory
  coordination for processes sharing a root; the portability fallback is
  process-local. Distributed locks, external file mutations, and custom event
  stores without this boundary remain outside the contract.
- `promote-redaction` can create a separate accepted custody record for a
  registered result. It verifies the source and result references and requires
  an accepted record anchoring the event's source digest before adding a
  `derived-from` parent. This preserves the original record and blob but does
  not claim that Lockwood transformed the payload.

## Used in

- [`lockwood/internal/custody`](../../lockwood/internal/custody/)
- [`lockwood append-event`](../../lockwood/cmd/lockwood/)
- [`Lockwood custody specification`](../../lockwood/CUSTODY-SPEC.md)

## Related

- [`A state label is not an executable transition`](../concepts/a-state-label-is-not-a-transition.md)
- [`A checksum-verified bundle proves artifact integrity, not isolation or correctness`](sorna-evidence-bundle.md)
