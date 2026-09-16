# Atomic replacement does not serialize independent store processes

An atomic file rename prevents partial reads, but only a shared lock prevents
two Hammond processes from losing one another's read-modify-write update.

## Origin

`FileStore` originally protected operations with an in-process mutex. That
mutex was sufficient for one store instance, but two instances using the same
directory could both read the same record and let the later rename erase the
earlier event.

## What

The file store now creates a `.hammond.lock` in its root and takes an advisory
exclusive lock for registration and all coordinated mutations. `Get` and
`List` take shared locks so readers do not observe the middle of an amendment
publication. The existing temporary-file-plus-rename write remains the guard
against partial record contents.

Callers that read before mutating can use `RecordRevision` with the conditional
append, amendment, and supersession operations. Hammond compares the revision
while holding the mutation lock and returns `ErrConflict` if another update
committed first.

The CLI exposes the same contract through `revision` and the optional
`--if-revision` flag, so an operator can make a stale-read failure visible
without needing to calculate the token in a separate integration.

## Why

The losing alternative was to rely on each `FileStore`'s `sync.Mutex`, which
does not coordinate processes and silently loses valid event history. A lock
serializes the local read-modify-write boundary without inventing a merge
policy for two semantically competing events. The revision precondition adds a
second signal: callers can distinguish a stale read from a lifecycle rejection
and deliberately retry or surface the conflict.

## Gotchas

- The lock coordinates Hammond writers; external programs that mutate record
  files directly are outside the contract.
- Serialization is not semantic conflict resolution. A later operation can
  still be rejected by lifecycle or lineage rules after it observes the first
  committed operation.
- A revision is derived from the materialized record and is not stored in the
  governance artifact; after `ErrConflict`, callers must read again rather than
  guess a replacement revision.
- The lock is advisory and depends on filesystem support for `flock`; it is a
  local-store mechanism, not a distributed coordination service.
- The lock file is operational metadata and is ignored by record listing.

## Used in

- `hammond/internal/store/filesystem.go`
- `hammond/internal/store/filesystem_test.go`
- `hammond/internal/store/store.go`
- `hammond/cmd/hammond/main.go`
- `hammond/cmd/hammond/main_test.go`

## Related

- [A governance state must name the policy that materializes it](hammond-review-policy-boundary.md)
- [A valid governance lineage is more than an acyclic graph](hammond-lineage-is-semantic.md)
