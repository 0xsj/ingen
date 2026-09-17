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
nublar run deliver --store <dir> --run-id <id> \
  --webhook https://example.test/nublar --timeout 30s \
  --receipt delivery-receipt.json \
  --receipt-store <receipt-dir>
```

The first reference publisher is a generic HTTP webhook in
`internal/delivery/webhook`. It POSTs the decision as JSON and sends the
`run_id` as the `Idempotency-Key` header. An optional HMAC-SHA256 signature is
sent as `X-InGen-Signature-256`; the CLI reads its secret from `--secret-env`
so secrets do not appear in shell arguments. It is covered with in-memory HTTP
transports only; no live endpoint is configured by Nublar.

The first concrete adapter is GitHub Checks in
`internal/delivery/githubchecks`. The `run deliver --transport github-checks`
command attaches a completed check run to a supplied repository and head SHA,
uses the Nublar `run_id` as the GitHub `external_id`, and reads a workflow token
from the environment. It maps coordinator errors to a failing GitHub check
while preserving Nublar's exact `error` status and exit code in the check
output. Its API behavior is covered by local HTTP contract tests; a live
repository acceptance passed in
[run 35204322692](https://github.com/0xsj/ingen/actions/runs/35204322692),
which created Check Run `105146774540` and an accepted HTTP `201` receipt.

The projection contains:

- the opaque `run_id`, logical workflow ID, and exact workflow file reference;
- optional provider-neutral external correlation (`system`, `id`, and
  positive `attempt`), kept separate from `run_id`;
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

## Delivery receipt

`run deliver --receipt <path>` optionally exports one versioned
`ingen.nublar-delivery-receipt/v1` record for the attempt. Its structural schema
is [`spec/receipt-v1.schema.json`](spec/receipt-v1.schema.json). The receipt
contains the run ID, transport, attempt timestamp, accepted/failed status, and
the HTTP status or error detail observed by the publisher. A failed webhook
attempt is written as a `failed` receipt before the command returns exit code
`2`, when the publisher was reached; configuration and projection failures
occur before a receipt exists. The receipt does not modify the stored run or
decision, and repeated deliveries produce separate exported receipts when
requested.

`run deliver --receipt-store <dir>` additionally persists every valid receipt
returned after a publisher attempt in a separate content-addressed local
store. `run receipt list --receipt-store <dir>` reads that history in
newest-first order. The receipt store is audit persistence only; it does not
implement delivery retries or change the run decision. Its filesystem contract
is documented in [`RECEIPT-STORAGE.md`](RECEIPT-STORAGE.md).

## Adapter rules

Future adapters may translate the projection into a provider's status,
annotation, webhook, or pull-request model, but they must not reinterpret
Sorna, Paddock, or another producer's report. `run_id` is the idempotency key
for delivering one local run to one destination; a rerun has a new run ID.

Delivery retries belong to the adapter. They must not create a new Nublar run
or change the stored decision. The `run deliver` command returns `0` when the
webhook accepts the projection and `2` for configuration, timeout, transport,
or non-2xx failures. A delivery failure is separate from the run's `status`
and should be reported through the optional receipt or adapter rather than
written back as a producer or coordinator result.

## Deliberately deferred

GitHub Checks is the first selected destination, but annotations, pull-request
comments, key rotation, a durable retry store, and other hosted providers
remain deferred. The generic webhook and GitHub Checks adapter both publish
only the provider-neutral projection; neither reinterprets producer-owned
reports.
