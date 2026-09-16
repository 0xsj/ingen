# Audit and emission must share one receipt snapshot

An audit decision can justify an envelope only when both are derived from the same validated receipt bytes.

## Origin

The CI command audited the receipt by one read and then loaded it again while building the shared envelope. A concurrent writer could insert an event or change status between those reads, leaving the audit summary attached to a different receipt report.

## What

`run.LoadFileSnapshot` returns the validated receipt and the exact bytes that produced it. Sentinel audit can consume that receipt directly, and CI-result construction can consume the same bytes directly. The report builder likewise audits the receipt value it will render.

## Why

The losing shape was “load wherever each helper needs it.” Separate reads make a path look like a stable identity when it is only a mutable name. One snapshot binds validation, the embedded report, and the receipt input hash to the same value without requiring a distributed lock for read-only consumers.

## Gotchas

- The snapshot protects receipt consistency, not the referenced workspace or artifact files; those are still verified at audit time and may require stronger storage policy for immutable retention.
- A snapshot is not an attestation that no writer changed the path afterward; the envelope’s receipt hash identifies the bytes consumed.
- Reported Sorna meaning remains opaque even when the receipt snapshot is exact.

## Used in

- [`herdr-sentinel/internal/run`](../../herdr-sentinel/internal/run/)
- [`herdr-sentinel/internal/audit`](../../herdr-sentinel/internal/audit/)
- [`herdr-sentinel/internal/report`](../../herdr-sentinel/internal/report/)
- [`sentinel run ci-result`](../../herdr-sentinel/cmd/sentinel/main.go)

## Related

- [`nublar-sentinel-verifier-workflow.md`](nublar-sentinel-verifier-workflow.md)
- [`sentinel-evidence-publication.md`](sentinel-evidence-publication.md)
- [`sentinel-receipt-update-lock.md`](sentinel-receipt-update-lock.md)
