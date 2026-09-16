# Nublar run identity

## Decision

`run_id` identifies one Nublar collection attempt. It is not the identity of
the workflow, a commit, or an external CI event.

- Nublar generates an opaque ID with cryptographic randomness when the caller
  does not provide one.
- A caller may provide a local ID for tests or imports. An external integration
  should use the optional `correlation` object for its authoritative external
  identifier.
- Collecting the same workflow again creates a new run ID. Nublar does not
  deduplicate runs by workflow contents, artifact contents, or timestamps.
- The filesystem store treats a run ID as immutable and rejects a duplicate
  save instead of overwriting the earlier record.

The workflow ID remains the logical workflow identity. The workflow file's
SHA-256 identifies the exact declaration used for that attempt. Status,
timestamps, check state, and artifact references belong to the individual run.

## Optional external correlation

An integration may add a provider-neutral `correlation` object with a
non-empty `system`, `id`, and positive `attempt`. This records which external
attempt requested the collection while keeping `run_id` as Nublar's immutable
identity for the local collection attempt. Nublar stores and projects these
values without interpreting provider-specific meanings.

## Deliberately deferred

Version `ingen.nublar-run/v1` does not yet model:

- `rerun_of` or `supersedes` relationships;
- branch, commit, pull request, or scheduler metadata.

Those fields should be introduced only with the first concrete integration
that needs them. They should be separate from `run_id`, so a provider's
correlation model cannot make local run identity ambiguous.
