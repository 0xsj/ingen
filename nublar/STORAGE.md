# Nublar run storage

## First storage boundary

The first durable storage contract is intentionally small:

```text
save validated run → publish exactly once
load run by ID    → validate before returning
list stored runs  → validate and return in deterministic order
```

The Go interface is [`internal/storage/store.go`](internal/storage/store.go),
with a local filesystem implementation in
[`internal/storage/filesystem`](internal/storage/filesystem).

## Filesystem behavior

The filesystem store:

- creates its configured root when saving the first run;
- derives the filename from SHA-256 of the opaque run ID, so IDs cannot escape
  the storage root through path syntax;
- writes to a temporary file and atomically publishes with a same-filesystem
  link after syncing the temporary file;
- rejects a second save for an existing run ID rather than overwriting history;
- validates the run before writing, after loading, and while listing.

Run loading uses the closed `ingen.nublar-run/v1` shape: unknown top-level
fields and multiple JSON values in one stored record are rejected before the
run reaches the storage or query surface.

The generated filename is an implementation detail. The run ID inside the
artifact remains the authoritative identity.

## Intentionally deferred

This boundary does not yet provide retention, garbage collection, locking
across processes, remote storage, or database migrations. Query filtering is a
read-side concern documented in [`RUN-QUERY.md`](RUN-QUERY.md). The remaining
storage capabilities should be added when a concrete consumer needs them.

The store protects readers from partial files and flushes file contents before
publication. Directory-level crash consistency and multi-process coordination
remain future hardening work.

The CLI's `--output` flag atomically publishes a user-selected export path.
The `run collect` command can additionally persist the same record in this
store without changing the producer envelope or the run schema. The output
contract is documented in [`OUTPUT-BOUNDARY.md`](OUTPUT-BOUNDARY.md).

Delivery receipts use a separate optional content-addressed store through
`--receipt-store`; they are documented in
[`RECEIPT-STORAGE.md`](RECEIPT-STORAGE.md) and are not mixed with run records.

The current CLI can opt into the store explicitly:

```sh
nublar run collect \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/runs \
  --output .artifacts/nublar-run.json

nublar run show --store .artifacts/runs --run-id <run-id>
nublar run list --store .artifacts/runs
```

Direct output is an export; `--store` is the durable, immutable record.
