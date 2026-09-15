# Nublar execution boundary

## Version 1 decision

`ingen.nublar-workflow/v1` is a collection declaration, not a process
execution plan. It says which producer envelopes must exist and where they
will be found after an external workflow has run.

For each check:

- `tool` is the producer identity expected in the shared CI envelope; it is
  not an executable name or shell command.
- `result` is the relative output path that the producer must publish under
  the artifact root.
- `required` is a collection and acceptance rule; it is not a scheduling
  instruction.

The external executor owns producer command resolution, arguments,
environment, workspace setup, sequencing or parallelism, timeouts, retries,
logs, and process exit handling. Sorna, Paddock, and future producers own the
meaning of their reports and findings.

Nublar owns the handoff after production:

```text
workflow file + artifact root
    → external executor runs producers
    → producers publish complete ingen.ci-result/v1 files
    → Nublar validates, hashes, preserves, and composes them
```

Nublar does not inspect a producer process, infer a result from a process exit,
retry a producer, or launch a command in this slice. If a required producer
does not publish a valid envelope, collection records an error. A producer
may publish a `failed` or `error` envelope; Nublar preserves that envelope and
composes its declared status.

## Publication handoff

An executor must use the workflow's artifact root and declared relative paths.
Result files should be written completely before they become visible to
Nublar. The executor may run independent checks in any order or in parallel;
the collection contract does not assign check ordering semantics.

The repository Makefile may compose producer targets before
`nublar run collect` as a local convenience. That composition is not part of
the Nublar workflow schema or the Nublar command's ownership boundary.

## Deliberately deferred

The workflow schema does not yet model producer commands, executor plugins,
timeouts, retry policy, scheduler state, logs, or process-level provenance.
Those belong behind a future `internal/execution` boundary and should be
added only when a concrete executor or hosted workflow requires them.
