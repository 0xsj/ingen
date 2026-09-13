# A fair mutation comparison requires the same sealed contract

A clean baseline and its mutation run are comparable only when they use identical sealed contract bytes and the same oracle configuration.

## Origin

The live clean and `status-200-create` runs both recorded the same
`document-pipeline` contract SHA-256 while differing only in the subject
variant.

## What

The contract hash anchors the behavioral claims being compared. A mutation run
can then attribute a changed result to the subject variant rather than to a
quiet contract edit. The mutation identity, adapter, and relevant runner
configuration must be retained alongside that hash.

## Why

If the contract changes between baseline and mutant, a newly failing or passing
rule cannot be attributed cleanly. Reusing the same file path is not enough:
the comparison needs the sealed bytes and their digest.

## Example

```text
clean contract hash:  5aca9dc3...
defect contract hash: 5aca9dc3...
subject variants:     live-clean / live-status-200-create
```

## Gotchas

- A matching contract hash proves artifact identity, not contract quality.
- The clean baseline should pass before mutation scoring begins.
- Different setup data, adapter behavior, or environment policy can still make
  two runs incomparable even when the contract hash matches.

## Used in

- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`live clean run`](../../.artifacts/live-clean-run/run.json)
- [`live defect run`](../../.artifacts/live-defect-run/run.json)
- the future Sorna mutation campaign

## Related

- [`Mutation results separate target sensitivity from setup fallout`](../modules/sorna-mutation-result-model.md)
- [`A killed mutation proves sensitivity, not correctness`](a-killed-mutation-proves-sensitivity.md)
- [`Go's JSON map ordering and canonical bytes`](../language/go-json-maps-and-canonical-bytes.md)
