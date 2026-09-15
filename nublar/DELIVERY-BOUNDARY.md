# Nublar delivery boundary

## Provider-neutral projection

The first delivery boundary is a projection, not a hosted integration. It
gives a future adapter the coordinator decision and enough references to
publish a useful status without transporting or interpreting producer-owned
reports.

The projection is versioned as `ingen.nublar-decision/v1`; its structural
schema is [`spec/decision-v1.schema.json`](spec/decision-v1.schema.json). It
can be exported from a stored run with:

```sh
nublar run decision --store <dir> --run-id <id> --output decision.json
```

The first reference publisher is a generic HTTP webhook in
`internal/delivery/webhook`. It POSTs the decision as JSON and sends the
`run_id` as the `Idempotency-Key` header. It is covered with local HTTP test
servers only; no live endpoint is configured by Nublar.

The projection contains:

- the opaque `run_id`, logical workflow ID, and exact workflow file reference;
- Nublar's `status` and `exit_code`;
- creation and completion timestamps;
- every check's ID, expected tool, declared path, required flag, status, and
  reason;
- exact result file references when a producer artifact was consumed;
- Nublar-owned warnings and errors.

It deliberately excludes the nested producer artifact and report. The full
`ingen.nublar-run/v1` record remains available through Nublar storage for a
destination that needs detailed evidence. The projection is implemented by
`internal/delivery.Project` and validates the source run first. The command
returns `0` when the projection is successfully read and written; the run's
decision remains in the projection's `status` and `exit_code` fields.

## Adapter rules

Future adapters may translate the projection into a provider's status,
annotation, webhook, or pull-request model, but they must not reinterpret
Sorna, Paddock, or another producer's report. `run_id` is the idempotency key
for delivering one local run to one destination; a rerun has a new run ID.

Delivery retries belong to the adapter. They must not create a new Nublar run
or change the stored decision. A delivery failure is separate from the run's
`status` and should be reported by the adapter rather than written back as a
producer or coordinator result.

## Deliberately deferred

No hosted provider, authentication model, annotation vocabulary, or delivery
retry store is selected yet. The generic webhook establishes transport shape
only; the first concrete consumer should establish those details behind this
projection.
