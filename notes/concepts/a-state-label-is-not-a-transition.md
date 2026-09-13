# A state label is not an executable transition

A contract can name the state a rule expects, but Sorna can execute it only when the contract also defines how the state is reached.

## Origin

The first Sorna runner encountered `given.state` on the document status,
processing, and result rules. The label communicated intent to a reader but did
not provide a replayable setup sequence, which led to the addition of
`given.setup` in the contract MVP.

## What

An executable stateful case needs at least a state identifier, a sequence of
public actions that produces it, and the observation or invariant that proves
the setup succeeded. In the current MVP, `given.setup` contains named request
steps, each with an expected observation and optional `body.field` captures.
The setup must use the same public boundary as the rule under test unless the
contract explicitly declares a fixture mechanism.

## Why

Guessing setup from a state name would couple Sorna to one subject's internal
model. Treating the rule as a standalone request would test a different
scenario. The runner now executes the declared setup and records each setup
observation; a setup failure leaves the target rule `inconclusive`.

## Example

```yaml
given:
  state: document_queued
  setup:
    - id: accept-document
      request:
        method: POST
        path: /documents
        body:
          name: welcome.md
          content: Read the contract.
      expect:
        status: 202
      capture:
        document_id: body.id

subject: GET /documents/{document_id}
expect:
  status: 200
```

This is the supported state-sequence shape used by the document-pipeline
contract.

## Gotchas

- Setup requests can have side effects, so their observations must be recorded
  rather than treated as invisible preparation.
- A setup that relies on private database state weakens the language-neutral
  boundary and can make cross-language subjects behave differently.
- State names should describe observable conditions, not implementation types
  such as `QueueEntry` or `ProcessedDocumentRow`.

## Used in

- [`document-pipeline contract`](../../examples/document-pipeline-lab/contract/contract.yaml)
- [`sorna/internal/runner`](../../sorna/internal/runner/)
- the current document-pipeline state-sequence rules

## Related

- [`The first Sorna runner accepts a subject URL instead of a subject package`](../modules/sorna-http-runner.md)
- [`The contract can cross language boundaries`](the-contract-can-cross-language-boundaries.md)
- [`Sorna contract specification`](../../sorna/CONTRACT-SPEC.md)
