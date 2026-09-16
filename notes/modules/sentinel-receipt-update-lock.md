# A receipt callback update needs a local lock around the read-modify-publish cycle

Atomic rename prevents a reader from seeing a half-written receipt, but it
does not prevent two callback processes from overwriting each other's events.

## Origin

The Herdr event batch adapter made one delivery all-or-nothing in memory. A
second delivery could still start another process, load the same old receipt,
append its own event, and publish after the first process. The last writer
would silently erase the first writer's accepted event. The same failure was
possible when an in-place artifact or Sorna lifecycle update raced a callback
update.

## What

`run.UpdateFile` acquires a sibling advisory lock, loads the current receipt,
executes one update against that in-memory value, and atomically publishes the
new receipt only when the callback reports a change. Sentinel's in-place Herdr
event, batch, artifact, oracle, and verifier lifecycle updates use this seam,
including when an explicit `--output` path resolves to the receipt path.
Idempotent replays therefore do not rewrite the receipt, while separate local
deliveries serialize their read-modify-publish cycles.

## Why

Locking only the final rename is too late: both writers can make decisions from
the same receipt state. Locking the whole cycle preserves the append-only
assumption at the local file boundary without moving receipt ownership into
Nublar or turning a filesystem lock into a distributed queue.

## Gotchas

- The lock is advisory and local to the filesystem; every writer must use the
  Sentinel update seam for it to help.
- The sibling `.lock` file is retained so its path remains stable across
  receipt replacement.
- This is not remote durability, authentication, or crash-recovery policy for a
  future Herdr host.
- A distinct output path intentionally remains snapshot-style: it reads the
  source receipt, applies the update in memory, and publishes a separate file.

## Used in

- `herdr-sentinel/internal/run/update.go`
- `herdr-sentinel/internal/run/lock_unix.go`
- `herdr-sentinel/cmd/sentinel` Herdr event, artifact, oracle, and verifier commands

## Related

- [`sentinel-herdr-event-batch.md`](sentinel-herdr-event-batch.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
