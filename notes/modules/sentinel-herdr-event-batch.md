# A Herdr event batch must publish atomically

A callback stream must not leave a Sentinel receipt partially updated when a
later event is malformed or violates the run boundary.

## Origin

The single-event adapter made callback identity, artifact references, and
receipt status explicit. The next failure mode was a host delivering several
callbacks together: writing each one directly could preserve the first event
while rejecting the second, leaving the durable receipt dependent on where the
stream stopped.

## What

`sentinel adapter herdr-events` reads newline-delimited
`ingen.herdr-event/v1` objects, validates the complete stream, applies the
events to an in-memory receipt copy, and publishes the receipt only after the
whole batch succeeds. Empty lines are ignored and errors identify the source
line. Identical event IDs remain idempotent, so a retried batch can contain
events already present in the receipt.

## Why

The alternative—loading and saving the receipt once per callback—creates a
partial-commit boundary inside one host delivery. A malformed event, a run or
workspace mismatch, or a drifted artifact after an earlier event would leave a
receipt that no longer represents one accepted delivery unit. Copy-then-publish
keeps the local adapter deterministic without claiming queue durability or
solving cross-process locking before Herdr's host persistence contract exists.

## Gotchas

- The batch is not an authenticated event log; it only validates the supplied
  shape and the Sentinel-owned bindings.
- Artifact files are checked under `--root` before events that reference them
  are accepted.
- The batch does not infer receipt status from event type. A host must carry an
  explicit `receipt_status` when it wants the operator-visible state updated.

## Used in

- `herdr-sentinel/internal/adapter`
- `herdr-sentinel/cmd/sentinel` (`adapter herdr-events`)
- `herdr-sentinel/spec/herdr-event-v1.schema.json`

## Related

- [`sentinel-herdr-event-adapter-boundary.md`](sentinel-herdr-event-adapter-boundary.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`nublar-sentinel-verifier-workflow.md`](nublar-sentinel-verifier-workflow.md)
