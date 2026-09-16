# Artifact identity makes retries safe without accepting replacement bytes

An artifact handoff can be retried only when its complete identity is unchanged; reusing an ID for different bytes or metadata must remain an error.

## Origin

The receipt lock made concurrent artifact writers safe, but a retried `run artifact` command still encountered the existing ID as a duplicate. Treating every retry as an error made recovery noisy, while blindly replacing the reference would break provenance.

## What

Sentinel registration compares an incoming artifact’s ID, role, kind, path, and SHA-256 with the existing reference. An exact match is an idempotent no-op; a new ID is appended after normal receipt validation; a reused ID with any difference is rejected as a conflict.

## Why

The losing alternatives were strict duplicate rejection for every call and replacement by ID. Strict rejection makes at-least-once delivery hard to recover; replacement allows a later file or producer to rewrite what an earlier event meant. Exact identity preserves both retryability and the append-only provenance model.

## Gotchas

- A same-path retry is not enough: the digest and producer metadata must also match.
- A changed file at the same path conflicts with the recorded artifact instead of updating it.
- Idempotent registration does not make a surrounding lifecycle event idempotent; event identity remains a separate concern.

## Used in

- [`herdr-sentinel/internal/run`](../../herdr-sentinel/internal/run/)
- [`sentinel run artifact`](../../herdr-sentinel/cmd/sentinel/main.go)
- [`sentinel-receipt-update-lock.md`](sentinel-receipt-update-lock.md)

## Related

- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`sentinel-herdr-event-adapter-boundary.md`](sentinel-herdr-event-adapter-boundary.md)
