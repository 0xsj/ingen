# Nublar status

Status: local alpha complete; implementation frozen pending a new consumer
requirement, 2026-09-17

Nublar has completed and verified its current consumer-led scope. It is a
local, filesystem-backed coordinator that collects producer-written
`ingen.ci-result/v1` envelopes, records immutable runs, projects provider-
neutral decisions, and records independent delivery receipts.

## Complete and verified

- Strict workflow, envelope, run, decision, and receipt contracts.
- Exact workflow and consumed-artifact SHA-256 provenance.
- Immutable filesystem run and receipt history with deterministic queries.
- Provider-neutral delivery plus generic HTTP webhook publishing.
- GitHub Checks delivery with token injection, status mapping, idempotent
  same-run update, bounded transient retries, and receipt persistence.
- GitHub Actions producer-to-Nublar handoff with separate producer and
  consumer jobs.
- Compatibility review retaining `aggregate` for existing users and directing
  new integrations to `run collect`.
- Storage and execution reviews confirming that hosted storage and producer
  orchestration are not currently required.

The canonical local gate is:

```sh
make nublar-freeze-check
```

The live GitHub Checks acceptance proof is
[workflow run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692).
It created Check Run `105146774540` with `completed/success` and persisted an
accepted `github-checks` receipt with HTTP status `201`.

## Current boundaries

- Nublar consumes producer envelopes; it does not launch or retry producers.
- Producer reports remain opaque and producer-owned.
- Filesystem history is local to the workflow or runner that owns it.
- `aggregate` remains a compatibility surface; it is not the preferred path
  for new integrations.
- GitHub Checks is the selected concrete delivery adapter; comments,
  annotations, approvals, and additional providers remain out of scope.

## Reopen conditions

Use [`CONSUMER-REQUEST-TEMPLATE.md`](CONSUMER-REQUEST-TEMPLATE.md) before any
new implementation. The request must identify the consumer, the insufficient
current behavior, required acceptance proof, and the impact on the frozen
contracts.

Without a new requirement, do not add hosted Nublar services, remote storage,
producer scheduling, SDKs, retry queues, or provider-specific features.
