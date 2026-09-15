# Nublar run identity

## Decision

`run_id` identifies one Nublar collection attempt. It is not the identity of
the workflow, a commit, or an external CI event.

- Nublar generates an opaque ID with cryptographic randomness when the caller
  does not provide one.
- A caller may provide an ID for tests, imports, or an integration that already
  has an authoritative identifier.
- Collecting the same workflow again creates a new run ID. Nublar does not
  deduplicate runs by workflow contents, artifact contents, or timestamps.
- The filesystem store treats a run ID as immutable and rejects a duplicate
  save instead of overwriting the earlier record.

The workflow ID remains the logical workflow identity. The workflow file's
SHA-256 identifies the exact declaration used for that attempt. Status,
timestamps, check state, and artifact references belong to the individual run.

## Deliberately deferred

Version `ingen.nublar-run/v1` does not yet model:

- external CI provider IDs or a cross-system correlation ID;
- attempt numbering;
- `rerun_of` or `supersedes` relationships;
- branch, commit, pull request, or scheduler metadata.

Those fields should be introduced only with the first concrete integration
that needs them. They should be separate from `run_id`, so a provider's
correlation model cannot make local run identity ambiguous.
