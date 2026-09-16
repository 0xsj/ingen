# Nublar output boundary

## CI-facing behavior

Nublar exposes a machine-readable JSON artifact and a process exit code. The
status mapping is shared across the aggregate and run paths:

```text
passed → 0
failed → 1
error  → 2
```

`run collect` persists and/or writes the complete run record before returning
the run's decision code. `run show` returns the stored run's decision code.
`run list` is a read operation and returns `0` when the store was read and the
JSON list was written successfully, regardless of the statuses of listed
runs. `run decision` likewise returns `0` when its provider-neutral projection
is successfully written; the projected run status remains in the JSON.
`run deliver` returns `0` only when the webhook accepts the projection and
`2` for delivery failures. With `--receipt`, it atomically exports the
independent delivery-attempt outcome, including failed attempts when the
publisher was reached. Storage, validation, serialization, and delivery
failures return `2`.

## File publication

Every user-selected `--output` path is an export of a fully validated JSON
value. Nublar writes the complete bytes to a temporary file in the destination
directory, syncs and closes it, then atomically replaces the requested path.
An unsuccessful write does not intentionally replace an existing output.
Parent directories must already exist; repository targets create their output
directories before invoking Nublar.

Stdout remains a stream for callers that do not provide `--output`. Nublar
does not contact a network endpoint unless `run deliver` is explicitly
invoked, and `--receipt` is also an atomic JSON export rather than a second
run record. Nublar does not emit provider-specific delivery formats in this
slice.

## Deliberately deferred

Hosted pull-request statuses, annotations, authentication, retries, output
signing, and provider-specific remote delivery belong behind the
provider-neutral projection documented in [`DELIVERY-BOUNDARY.md`](DELIVERY-BOUNDARY.md)
and future concrete adapters. The generic webhook transport is the limited
reference publisher for this slice.
