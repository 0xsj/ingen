# The contract can cross language boundaries

A public behavioral contract can evaluate implementations in different languages, but the adapter and mutation strategy still belong to the subject.

## Origin

The first document-pipeline lab discussion: deciding whether the example
subject had to be written in Go because Sorna is being built in Go.

## What

When Sorna evaluates a public HTTP/JSON boundary, it needs the contract,
inputs, observations, and rules—not the implementation language. A Go service,
Python service, and TypeScript service can all be subjects of the same sealed
contract if they expose the same behavior at that boundary.

## Why

Making the contract language-specific would reproduce the closed loop at a
different layer: the oracle would begin asserting framework or implementation
details instead of product behavior. Keeping the contract language-neutral
makes the same behavioral claim portable and lets a finding follow the subject
when the implementation is rewritten.

The language still matters outside the contract. An adapter must know how to
start and observe the subject, a build provider must know how to compile it,
and source-level mutation providers are necessarily language-specific. Those
are integration concerns, not contract semantics.

## Example

The document pipeline contract can describe `POST /documents` and the required
state transitions without saying whether the subject uses Go's `net/http`, a
Python web framework, or a Node server.

## Gotchas

- A public contract can still accidentally encode framework behavior through
  unstable error text, internal status conventions, or generated fields.
- A behavioral mutation can be language-neutral, while applying that mutation
  to source code is not.
- An application SDK may provide useful observations, but the sandbox must not
  treat self-reported instrumentation as stronger than public observations or
  host-enforced access evidence.

## Used in

- [`examples/document-pipeline-lab`](../../examples/document-pipeline-lab/)
- Sorna's future HTTP/JSON adapter
- Future webhook and invitation/access labs

## Related

- [`sorna/CONTRACT-SPEC.md`](../../sorna/CONTRACT-SPEC.md)
- [`sorna/ORACLE-SPEC.md`](../../sorna/ORACLE-SPEC.md)
- [`ISOLATION-THREAT-MODEL.md`](../../ISOLATION-THREAT-MODEL.md)
- [`NOTES.md`](../../NOTES.md)
