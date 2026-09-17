# Sentinel's native Herdr binding needs a host contract first

## Claim

The provider-neutral `ingen.herdr-event/v1` ingress is ready, but a native
Herdr plugin cannot be implemented responsibly from Sentinel alone. The host
must first expose callback, identity, durability, and failure semantics. This
note is the intake contract for that future binding; it is not a proposed
Herdr SDK.

## Required host inputs

| Host primitive | Minimum information Sentinel needs | Why it matters |
| --- | --- | --- |
| Lifecycle hook | Which host actions produce a callback, when it fires, and whether callbacks may be duplicated or reordered | Determines event normalization and retry behavior. |
| Run/workspace identity | Stable host run ID, workspace ID, workspace version, and role/session identity across callback retries and host restarts | Binds a callback to exactly one Sentinel receipt. |
| Delivery acknowledgement | Whether the host receives success, rejection, or retryable failure, plus its retry/backoff policy | Prevents an adapter error from becoming an invisible dropped event. |
| Persistence boundary | Which side durably owns event identity, receipt updates, and recovery after restart | Defines whether Sentinel's local lock/file path is sufficient or a host store is required. |
| Artifact handoff | Stable artifact reference, path namespace, byte-availability timing, and immutability/hash semantics | Lets Sentinel verify bytes instead of trusting a callback's claimed hash. |
| Callback origin | Host-provided authentication or provenance that the plugin can validate | Separates host authenticity from Sentinel's structural validation. |
| Cancellation and shutdown | In-flight callback cancellation, plugin unload, and host shutdown behavior | Keeps partial lifecycle updates and retry decisions explicit. |

The host may provide more than this, but it must not require Sentinel to infer
identity from pane text, process exit alone, filenames, or an untrusted report.

## Host contract handoff format

The Herdr owner can close this intake by supplying the following answers from
the real host implementation. An unknown or deferred field keeps the native
binding blocked; Sentinel must not fill it with an assumed default.

```text
Host implementation and version:
Hook name(s) and registration lifetime:
Callback payload and event identity:
Ordering, duplication, and delivery timing:
Run/workspace/role/session identity across retries and restarts:
Acknowledgement outcomes and retry/backoff policy:
Durable owner for event IDs and receipt updates:
Restart recovery and duplicate-delivery behavior:
Artifact reference namespace and byte-availability timing:
Artifact immutability and hash authority:
Callback authentication or provenance mechanism:
Cancellation, unload, and shutdown behavior:
Host documentation or executable fixture:
```

The handoff should identify which answers are normative, which are observed
behavior, and which remain unsupported. Once complete, it becomes the input to
the thin binding review; it does not change the provider-neutral Sentinel event
contract by itself.

The installed Herdr 0.9.0 surface is now recorded in
[`sentinel-herdr-v0-9-compatibility.md`](sentinel-herdr-v0-9-compatibility.md).
It supplies a usable plugin and event-hook surface, but the event envelope does
not supply the durable event identity, host timestamp, replay cursor, or
delivery/retry contract required for production Sentinel translation.

## Observed Herdr 0.9.0 probe evidence

The compatibility probe was enabled for a controlled live session and received
real `pane.agent_status_changed` callbacks. Herdr's plugin log recorded
successful invocations, and the live state directory contained 52 valid
captures at the time of review. The probe was disabled after capture so it does
not continue writing background diagnostics.

The latest raw envelope was structurally:

```json
{
  "event": "pane_agent_status_changed",
  "data": {
    "type": "pane_agent_status_changed",
    "pane_id": "wP:p6",
    "workspace_id": "wP",
    "agent_status": "blocked",
    "agent": "codex"
  }
}
```

The invocation context supplied workspace and tab labels, current working
directories, focused pane information, `invocation_source`, and
`correlation_id`. It did not supply a durable event ID or host event timestamp.
The probe's `captured_at` value remains local observation time and is not used
as either field. Ordering, replay, acknowledgement, retry, durable ownership,
authentication, artifact handoff, and shutdown behavior remain unanswered.

## Normalization boundary

The native binding should be a thin translation layer:

```text
Herdr hook payload
    → host binding validates host-owned identity/authentication
    → binding maps to ingen.herdr-event/v1
    → Sentinel adapter validates receipt/workspace/artifact lineage
    → Sentinel receipt update under its durability boundary
```

The binding must preserve the host event ID, run/workspace/session identity,
event time, role, outcome, reason, and artifact IDs. It must not add a new
behavioral verdict or reinterpret Sorna's report. Sentinel remains responsible
for normalized event validation, monotonic timestamps, artifact byte checks,
idempotent event identity, terminal lifecycle protection, and the shared CI
projection.

## Acceptance gate

The first native binding is ready for review only when it demonstrates all of
the following against the real Herdr hook or callback implementation:

1. A valid callback becomes one valid `ingen.herdr-event/v1` event with the
   active run/workspace/session identity preserved.
2. An identical delivery retry is a no-op; reusing an event ID with different
   content is rejected and surfaced to the host.
3. A callback for another run or workspace is rejected before receipt mutation.
4. An event that references an unknown, missing, drifted, or root-escaping
   artifact is rejected before receipt mutation.
5. An older callback timestamp and a terminal-to-running update are rejected;
   a terminal cleanup event follows the explicit cleanup rule.
6. A multi-event delivery cannot publish a partial receipt when a later event
   is malformed or conflicts.
7. Host restart/retry behavior is exercised, with the durable owner and
   recovery result documented.
8. Callback authentication/provenance is tested separately from the Sentinel
   structural audit; a passing audit must not be presented as proof of host
   authenticity.

The existing provider-neutral proofs cover items 1–6 at the Sentinel boundary,
including rejection of callbacks with the wrong run or workspace context,
missing or drifted artifact bytes, and conflicting event IDs before receipt
mutation, including through the locked same-path CLI update. Items 7–8 remain
intentionally unclaimed until Herdr supplies the corresponding host semantics.

## Explicit non-goals

- Defining Herdr's hook names, callback transport, or session model here.
- Adding a fake native SDK or pretending that `.gitkeep` is a plugin runtime.
- Moving workspace ownership into Nublar or verification semantics into
  Sentinel.
- Treating UI visibility, pane output, or self-reported agent activity as
  independent evidence.

## Related

- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
- [Herdr events should enter Sentinel, not Nublar](sentinel-herdr-event-adapter-boundary.md)
- [Sentinel's alpha boundary should freeze before native Herdr binding](sentinel-alpha-interface-checkpoint.md)
- [`ALPHA-INTERFACES.md`](../../ALPHA-INTERFACES.md)
