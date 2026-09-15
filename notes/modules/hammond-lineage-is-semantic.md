# A valid governance lineage is more than an acyclic graph

Hammond lineage validation must check event ownership and temporal governance
preconditions, not only whether predecessor and successor nodes form a graph
without cycles.

## Origin

The file store enforced amendment and supersession preconditions while writing,
but records imported from another backend were only checked for structural
links and cycles.

## What

An amendment event belongs to the predecessor record, and its successor must
already have a registration event. A supersession event also belongs to the
predecessor, and the successor must have an approval event at or before the
supersession timestamp and an existing amendment link. These checks are
performed over the records and event timestamps, independently of how the
records were persisted.

## Why

A graph can be acyclic while still claiming that a contract was superseded
before its successor was approved, or while placing a valid-looking amendment
edge on an unrelated record. Structural reachability is not evidence that the
governance sequence was valid.

## Gotchas

- A successor may later be superseded itself; its earlier approval still
  satisfies the predecessor's supersession precondition.
- Event timestamps are part of the temporal check, not display metadata.
- Store-time checks and imported-record checks must agree; neither is a
  substitute for the other.

## Used in

- `hammond/internal/governance/lineage.go`
- `hammond/internal/store`
- `hammond/cmd/hammond`

## Related

- [Hammond governance specification](../../../hammond/GOVERNANCE-SPEC.md)
- [Hammond approval governs identified bytes, not behavior](hammond-governance-artifact-identity.md)
