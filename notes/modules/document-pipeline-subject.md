# The document subject keeps workflow transitions visible at the boundary

The lab subject makes acceptance, processing, and result retrieval separate public transitions so an independent verifier can observe each state.

## Origin

The first implementation slice for `examples/document-pipeline-lab`: we needed
a real subject that Sorna can exercise without privileged access to its store.

## What

The subject stores documents in memory, returns `queued` from creation, changes
to `completed` only through the explicit process endpoint, and derives the
result from the stored content. The internal map is an implementation detail;
the observable workflow is the HTTP/JSON contract.

## Why

Processing during `POST /documents` would hide the queued state and make the
workflow less useful as a stateful verification example. Exposing the store or
adding test-only hooks would let the verifier assert implementation details
instead of public behavior.

## Example

```text
POST /documents  -> 202 queued
GET  /documents/{id} -> 200 queued
POST /documents/{id}/process -> 200 completed
GET  /documents/{id}/result -> 200 derived metadata
```

## Gotchas

- The in-memory store intentionally loses data when the process exits.
- Processing is synchronous in this first slice; the contract does not promise
  a real background queue.
- The contract still needs explicit rules for repeated processing and result
  retrieval before completion if those behaviors become important.

## Used in

- [`examples/document-pipeline-lab/subject`](../../examples/document-pipeline-lab/subject/)
- the future Sorna HTTP/JSON runner

## Related

- [`The contract can cross language boundaries`](../concepts/the-contract-can-cross-language-boundaries.md)
- [`A generated boundary must be finite and reproducible`](../techniques/a-generated-boundary-must-be-finite.md)
- [`document-pipeline contract`](../../examples/document-pipeline-lab/contract/contract.yaml)
