# An operator report must separate lifecycle, integrity, and producer meaning

A readable Sentinel report is useful only when it shows what Sentinel knows without turning a completed receipt or a verified hash into a Sorna verdict.

## Origin

The receipt and audit artifacts already held the facts needed to inspect a run, but they were split across JSON files and easy to collapse into a single “passed” label at the command boundary.

## What

`sentinel run report` renders the lifecycle receipt, the audit integrity result, ordered events, registered artifact identities, audit checks, and explicit limitations in one text view. Sorna’s result remains producer-owned and is named as such rather than inferred from lifecycle status.

## Why

The losing shape was a status-only summary. It hides whether “completed” means only that orchestration ended, whether referenced bytes still match, and whether a producer’s verification result was ever present. Keeping those dimensions visible makes the report useful for an operator while preserving the trust boundary between Sentinel and Sorna.

## Gotchas

- `completed` is a lifecycle state, not a behavioral pass.
- A failed audit can coexist with a completed receipt when referenced bytes drift or disappear.
- A passing audit verifies the receipt’s recorded bytes and structure at audit time; it does not attest to Herdr callback origin, unobserved access, or Sorna enforcement.

## Used in

- [`herdr-sentinel/internal/report`](../../herdr-sentinel/internal/report/)
- [`sentinel run report`](../../herdr-sentinel/cmd/sentinel/main.go)
- [`herdr-sentinel/README.md`](../../herdr-sentinel/README.md)

## Related

- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`sentinel-herdr-event-adapter-boundary.md`](sentinel-herdr-event-adapter-boundary.md)
- [`nublar-sentinel-verifier-workflow.md`](nublar-sentinel-verifier-workflow.md)
