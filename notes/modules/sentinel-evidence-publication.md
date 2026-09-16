# Evidence reports must publish as complete files

A validated Sentinel report is not a usable artifact until consumers can read one complete version of it.

## Origin

Receipt publication already used a temporary file and atomic rename, but audit JSON and operator text reports wrote directly to their final paths. An interrupted write could leave an invalid or partial artifact at a path that a later workflow expected to contain evidence.

## What

Sentinel audit and operator-report writers render into a sibling temporary file, flush and close it, then rename it into place. Readers therefore see the previous complete file or the new complete file at the final path, not an intermediate serialization.

## Why

The losing shape was direct writing to the final path. It exposes truncation and partial JSON/text to concurrent readers and makes a failed producer look like a malformed result without preserving the prior artifact. The temporary-file boundary matches receipt publication and keeps the final path a complete-file boundary.

## Gotchas

- Rename atomicity is a same-filesystem property; this does not provide remote retention or a signed attestation.
- File syncing reduces ordinary loss on process or system failure, but directory durability and storage policy remain outside this local slice.
- The report is operator-facing text; atomic publication protects readability, not the truth of self-reported Herdr activity.

## Used in

- [`herdr-sentinel/internal/audit`](../../herdr-sentinel/internal/audit/)
- [`herdr-sentinel/internal/report`](../../herdr-sentinel/internal/report/)
- [`sentinel run audit` and `sentinel run report`](../../herdr-sentinel/cmd/sentinel/main.go)

## Related

- [`sentinel-receipt-update-lock.md`](sentinel-receipt-update-lock.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
