# Nublar run artifact

## Proposed contract

The first durable Nublar run record is `ingen.nublar-run/v1`.

This is deliberately separate from the current `ingen.nublar-result/v1`
prototype. The prototype is an aggregate output used to prove the shared CI
envelope boundary. The run artifact is the record of one Nublar collection
attempt, with an identity, workflow reference, check-level state, provenance,
and final decision.

The machine-readable schema is [`spec/run-v1.schema.json`](spec/run-v1.schema.json).

The JSON schema defines the portable structural contract. Go validation adds
cross-field semantics such as producer/status matching, duplicate path
rejection, and aggregate severity; those semantics are covered by the Nublar
unit and integration tests.

## Shape

```json
{
  "schema": "ingen.nublar-run/v1",
  "run_id": "run-20260915T120000Z-01",
  "workflow": {
    "id": "document-pipeline-ci",
    "file": {
      "path": "nublar/workflows/document-pipeline.yaml",
      "sha256": "..."
    }
  },
  "status": "passed",
  "exit_code": 0,
  "created_at": "2026-09-15T12:00:00Z",
  "completed_at": "2026-09-15T12:00:01Z",
  "checks": [
    {
      "id": "behavioral-verification",
      "tool": "sorna",
      "path": "document-pipeline-ci-result.json",
      "required": true,
      "status": "passed",
      "result": {
        "ref": {
          "path": "document-pipeline-ci-result.json",
          "sha256": "..."
        },
        "artifact": {}
      }
    }
  ],
  "warnings": [],
  "errors": []
}
```

`artifact` is the complete producer-owned `ingen.ci-result/v1` envelope. The
`ref` binds that envelope to the exact file Nublar consumed. Nublar does not
replace the producer report with a summary of its own.

## Field semantics

| Field | Meaning |
| --- | --- |
| `schema` | The versioned Nublar run contract. |
| `run_id` | An opaque identity for this collection attempt; see [`RUN-IDENTITY.md`](RUN-IDENTITY.md). |
| `workflow` | The workflow ID and exact workflow file reference used. |
| `correlation` | Optional external system, correlation ID, and positive attempt number; separate from `run_id`. |
| `status` | Coordinator decision: `passed`, `failed`, or `error`. |
| `exit_code` | Shared decision mapping: `0`, `1`, or `2`. |
| `created_at` | When Nublar created the run record. |
| `completed_at` | When collection and decision making finished. |
| `checks` | One entry for every declared workflow check. |
| `warnings` | Non-blocking collection issues, such as a missing optional result. |
| `errors` | Blocking Nublar-owned collection or validation issues. |

Each check preserves its declared `path` even when the result is missing. This
keeps optional and failed collection attempts reviewable without requiring the
workflow file to be reopened.

Each check has one of these states:

- `passed`: a producer envelope was consumed with status `passed`.
- `failed`: a producer envelope was consumed with status `failed`.
- `error`: a producer envelope reported `error`, or Nublar could not consume
  the declared result.
- `missing`: an optional result was not present.

An `error` check may contain a producer artifact or a Nublar-owned `reason`.
A `missing` check contains a `reason` and no producer artifact.

## Decision rules

For present producer artifacts, Nublar composes only the envelope status:

```text
error > failed > passed
```

A missing required result, malformed result, duplicate path, or producer
identity mismatch makes the run `error` with exit code `2`. A missing optional
result is a warning and does not change an otherwise passing decision. A run
with no present result artifacts is an error even if every declared check was
optional.

## Compatibility boundary

The existing `ingen.nublar-result/v1` aggregate remains valid for the current
CLI and compatibility tests. The `nublar run collect` command emits
`ingen.nublar-run/v1` while the prototype `aggregate` command remains available
during the transition.

The run schema intentionally does not define producer commands, scheduling,
pull-request delivery, retention, or a remote artifact store. Those belong to
later Nublar capabilities and should consume this record rather than change
producer semantics.

## Collection review notes

The following invariants are enforced by the collector and its tests:

- Result paths must be retained on every check, including missing optional
  checks, and duplicate paths must be rejected after normalization.
- A result's bytes must be read, validated, and hashed from the same in-memory
  contents. Loading and hashing the path in separate reads would weaken the
  provenance claim through a time-of-check/time-of-use window.
- Only a missing optional file is a warning. A present but malformed optional
  result is a blocking collection error.
- A run with no accepted producer artifact is representable as an error run
  when its collection errors are recorded.
- Run IDs are opaque and unique per collection attempt. Optional external
  correlation is carried separately and does not change run identity; rerun
  relationships remain deferred. See [`RUN-IDENTITY.md`](RUN-IDENTITY.md).
- User-selected output paths and durable storage must publish complete records;
  the current atomic publication behavior is documented in
  [`OUTPUT-BOUNDARY.md`](OUTPUT-BOUNDARY.md) and [`STORAGE.md`](STORAGE.md).
