# Nublar run queries

## First read contract

The first filesystem-backed read surface has two operations:

```text
run show --store <dir> --run-id <id>  → one validated run record
run list --store <dir>                → all validated run records
run list --store <dir> --status failed --workflow document-pipeline-ci
                                      → matching validated run records
run list --store <dir> --external-system github-actions \
         --external-id build-42 --attempt 2
                                      → one external attempt, when present
```

`run show` addresses one immutable record by its opaque ID, writes the
validated record, and returns that record's stored decision exit code. `run
list` emits a JSON array of complete `ingen.nublar-run/v1` records, including
the preserved producer artifacts. It orders records newest first by
`created_at`, with ascending `run_id` as the deterministic tie-breaker.

An uncreated or empty store lists as `[]`. Temporary files and files that do
not have the canonical content-addressed filename shape are ignored. A
canonical run file that cannot be parsed, validated, or matched to its
filename fails the entire list operation rather than being silently omitted.

Listing is a read success regardless of the status of an individual run, so a
stored `failed` or `error` run does not make `run list` return a failure exit
code. Storage and serialization errors still return exit code `2`.

`run list` may filter by the coordinator `--status` (`passed`, `failed`, or
`error`), exact `--workflow` ID, exact `--external-system`, exact
`--external-id`, and positive `--attempt`. Correlation filters are independent
and composable; a run without correlation does not match an external filter.
Filters are applied after every stored record has been validated, preserve the
documented ordering, and return `[]` when nothing matches. An unsupported
status or negative attempt filter is a usage error with exit code `2`.

## Deliberately deferred

The first query surface does not define pagination, summary-only responses,
retention queries, or a hosted API. Those should be added when a consumer
establishes the required scale and response shape.
