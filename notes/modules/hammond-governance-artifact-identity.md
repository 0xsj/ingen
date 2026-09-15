# Hammond approval governs identified bytes, not behavior

A Hammond approval is meaningful only when it is bound to the exact contract
artifact digest, and it must not be presented as proof that Sorna verified the
contract's behavior.

## Origin

The first Hammond governance model needed to refer to Sorna contracts without
duplicating Sorna's contract semantics or verification evidence.

## What

Hammond records who reviewed a contract version, what decision they made, and
which canonical artifact bytes they reviewed. The contract path or URI helps a
consumer find the artifact, but the SHA-256 digest identifies the bytes. The
governance state is therefore separate from Sorna's contract status and from
Sorna's run verdicts.

## Why

A path can be replaced while retaining the same name, and a logical version can
be amended without preserving the same bytes. Approving only a path, ID, or
version would allow a later artifact to inherit an earlier decision silently.
Conversely, treating approval as verification would collapse a human governance
decision and an independent behavioral result into one unsupported claim.

The event history is retained instead of storing only the current state because
`approved` or `superseded` does not explain which review decision produced it or
which amendment changed the lineage.

The materialized state must be produced by the governance transition itself.
The file store may persist a snapshot, but it must not invent or update a state
label while appending an event; otherwise persistence can silently disagree
with the lifecycle rules.

An amendment touches two records: the new successor and the predecessor's
lineage event. The store must coordinate those writes or recover from a failed
second write; otherwise a partially published amendment can make the registry
look like it contains a lineage edge whose target does not exist.

Cross-record lifecycle events must not be accepted through a generic append
path. Amendment and supersession operations need identity and state checks that
an isolated event append cannot perform.

## Example

An approval for `document-pipeline@1` is valid only when its decision event
contains the same digest as the registered contract artifact. A version 2
amendment links back to version 1, but version 2 needs its own review and
approval even if the change is clarifying.

## Gotchas

- A URI is a locator, not an identity.
- A matching digest establishes byte integrity, not contract correctness.
- Hammond lifecycle state must not replace Sorna's `draft`, `sealed`, or
  `superseded` contract status.
- An amendment must preserve the predecessor record; it must not rewrite its
  review history.

## Used in

- `hammond/GOVERNANCE-SPEC.md`
- `hammond/internal/governance`
- `hammond/internal/store`
- `hammond/spec/ingen.hammond-governance-v1.schema.json`

## Related

- [Sorna contract specification](../../../sorna/CONTRACT-SPEC.md)
- [A state label is not an executable transition](../concepts/a-state-label-is-not-a-transition.md)
- [Hammond governance specification](../../../hammond/GOVERNANCE-SPEC.md)
