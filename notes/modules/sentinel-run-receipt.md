# A Sentinel receipt connects lifecycle events to opaque artifacts

Sentinel needs a durable record of the workflow around Sorna and Nublar. The
record must make the run reviewable without becoming a second verifier.

## Origin

The workspace manifest named roles and capability paths, but it did not record
which run created the workspace, which role produced an artifact, or when a
handoff occurred.

## What

`ingen.sentinel-run/v1` records a run ID, a hash of the exact workspace
manifest, an operator-visible lifecycle status, ordered events, and optional
artifact references. Artifact references carry a local SHA-256 and are linked
from events by ID. The first bootstrap command writes a receipt containing a
`workspace-created` event.

## Why

The lifecycle is a Sentinel concern: it connects workspaces, roles, policies,
and downstream tools. Sorna still owns behavioral results and isolation
evidence, while Nublar still owns workflow collection and delivery decisions.
The receipt preserves those artifacts as opaque references instead of copying
or summarizing their meaning.

## Gotchas

- A receipt hash binds a file reference to bytes read; it does not prove who
  had access to those bytes.
- Event ordering and timestamps are validated, but a status label is not an
  executable state transition.
- The current bootstrap command records only workspace creation. Real role
  launch, policy, Sorna, review, and cleanup events need a Herdr adapter or a
  future event-ingestion command.
- Local relative paths keep the first receipt portable inside the project;
  remote retention and signed attestations remain outside this slice.

## Result

```sh
make sentinel-run-bootstrap
```

This produces `.artifacts/sentinel-webhook-run.json`, a validated
`ingen.sentinel-run/v1` receipt with the workspace manifest's exact SHA-256.

## Used in

- `herdr-sentinel/internal/run`
- `herdr-sentinel/cmd/sentinel`
- `herdr-sentinel/spec/run-v1.schema.json`
- `make sentinel-run-bootstrap`

## Related

- [`sentinel-contract-workspace.md`](sentinel-contract-workspace.md)
- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
- [`nublar-input-provenance.md`](nublar-input-provenance.md)
