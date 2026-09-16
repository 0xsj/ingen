# Herdr events should enter Sentinel, not Nublar

The next integration boundary is a Herdr event adapter. The repository does
not yet expose Herdr plugin hooks or a host event API, so the local adapter
defines the translation contract without claiming to be a live listener.

## Ownership

The adapter belongs in the Herdr Sentinel plugin integration layer, adjacent
to `herdr-sentinel/plugin` and the Sentinel lifecycle service. It should not
live in Nublar's delivery package, because Nublar does not own workspace or
role lifecycle. It should not live in Sorna, because Sorna owns verification
and enforcement evidence. Hammond remains the owner of contract governance
events and approvals.

The flow is:

```text
Herdr host lifecycle hook
    → Sentinel event adapter
    → validated ingen.sentinel-run/v1 receipt event
    → Sentinel shared CI envelope when a terminal result is requested
    → Nublar workflow collection
```

## Normalized event contract

The host-facing adapter should normalize each event to the existing Sentinel
event fields before calling the receipt domain:

- the active Sentinel `run_id` and host event identity;
- event type, timestamp, role, and logical workspace/session reference;
- hashed artifact references when an artifact is produced;
- outcome and reason, including blocked or policy-violation explanations.

The adapter must verify that the event targets the loaded receipt's run and
workspace, reject unknown event types or out-of-order transitions, and fail
closed when an artifact reference is not already present in the receipt. The
artifact-registration path remains responsible for hashing and byte-level
verification; this event ingress does not trust a callback to introduce a new
file hash. Rooted verification also resolves the registered path and rejects a
symlink escape outside the supplied root. The same rooted resolver is used by
the Sentinel audit, so the terminal CI gate and event ingress apply one
containment rule. Workspace bootstrap and artifact registration use that
resolver when establishing new file references as well. Pane text, agent
self-report, and process exit alone must not be promoted to a verified Sorna
result.

The current alpha transition guard is intentionally narrow: event timestamps
must be monotonic, and a terminal receipt cannot regress to a non-terminal
status. This rule lives in the receipt domain so direct Sorna lifecycle writes
cannot bypass it. Cleanup may move a terminal receipt to `cleaned`; the full
Herdr state-machine graph remains unfrozen until the host lifecycle contract
exists.

The local proof is:

```sh
sentinel adapter herdr-event \
  --receipt .artifacts/sentinel-webhook-run.json \
  --event .artifacts/herdr-event.json \
  --output .artifacts/sentinel-webhook-run-updated.json
```

The event input is versioned as
[`herdr-event-v1.schema.json`](../../herdr-sentinel/spec/herdr-event-v1.schema.json).
The adapter requires a stable `event_id`, binds the event to the receipt's
run/workspace identity, and treats identical replays as idempotent no-ops.
Reusing an event ID with different content is rejected. The CLI regression also
checks the durable boundary: an identical retry leaves the receipt bytes
unchanged, so idempotency is not merely an in-memory event-count property.

The failed-terminal path is covered as well: a `sorna-completed` callback with
`receipt_status: failed` closes the receipt, an identical retry is a no-op, and
a later callback attempting to reopen it as `running` is rejected. This keeps
Herdr retry behavior separate from lifecycle reopening.

## Output rules

The adapter updates only the Sentinel-owned `ingen.sentinel-run/v1` receipt.
Nublar sees the receipt only through Sentinel's `ingen.ci-result/v1` adapter;
the receipt remains opaque report data with exact input lineage. A Herdr event
must never mutate a stored Nublar run or delivery receipt directly.

The first host implementation should support append-only delivery, durable
event identity, and explicit rejection/error reporting. Queues, remote event
stores, replay policy, and host-specific authentication should wait for the
actual Herdr plugin contract.

## Current status

The Sentinel receipt already provides strict event ordering, artifact linkage,
and terminal lifecycle validation. The local `sentinel adapter herdr-event`
command now provides the executable ingress seam. The missing piece is the
host binding: until Herdr exposes its callback and persistence semantics, no
live plugin listener or remote event store is claimed.

## Related

- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`nublar-sentinel-verifier-workflow.md`](nublar-sentinel-verifier-workflow.md)
- [`hammond-review-policy-boundary.md`](hammond-review-policy-boundary.md)
