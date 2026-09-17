# Sentinel's Herdr 0.9.0 compatibility boundary

## Observed host surface

The installed host is `herdr 0.9.0`. Its bundled API schema reports protocol
22 and schema version 1. The host provides the plugin manifest, event hooks,
plugin context environment, CLI wrappers, and newline-delimited JSON socket
API needed for a first compatibility probe.

The installed schema's event envelope is:

```json
{
  "event": "pane_agent_status_changed",
  "data": {
    "type": "pane_agent_status_changed",
    "workspace_id": "w1",
    "pane_id": "w1:p1",
    "agent_status": "working"
  }
}
```

The event kinds include workspace, worktree, tab, pane, and layout lifecycle
events. Plugin event hooks receive `HERDR_PLUGIN_EVENT` and
`HERDR_PLUGIN_EVENT_JSON`; available workspace, tab, and pane IDs are injected
separately, and the full invocation context is available through
`HERDR_PLUGIN_CONTEXT_JSON`.

The official plugin surface also documents that event commands run
asynchronously and their completion is recorded in the plugin command log.
Plugin registration and state directories persist across restarts, but the
plugin owns the state format, migrations, and cleanup. These facts establish a
useful local observation and storage surface; they do not establish a
per-callback acknowledgement, retry decision, event cursor, or crash-recovery
protocol for Sentinel.

## Compatibility gaps

The current host surface does not yet satisfy Sentinel's production event
contract by itself:

- the event envelope has no host-issued `event_id` or event timestamp;
- lifecycle subscriptions begin at acknowledgement and do not replay events
  retained before subscription;
- plugin event commands are logged and run by the host, but the public surface
  does not define a Sentinel-specific delivery acknowledgement, retry, or
  durable event cursor; and
- Herdr provides plugin config/state paths, but the plugin owns the file format
  and recovery policy.

These gaps matter because Sentinel's `ingen.herdr-event/v1` requires durable
event identity, monotonic event time, idempotent retries, and a defined receipt
recovery boundary. A probe-local capture time or a hash of the payload must not
be silently promoted to host event identity.

## Probe

The version-pinned probe lives in [`herdr-sentinel/plugin`](../../herdr-sentinel/plugin/).
It captures the raw host envelope and context without appending to a Sentinel
receipt. Link it only in a controlled development session, then inspect the
captured payloads under the plugin state directory.

The probe's purpose is compatibility evidence, not production lifecycle
translation. Once the host owner supplies event identity, timestamp, retry,
replay, and recovery semantics, the production binding can map the observed
fields into the existing provider-neutral adapter.

The repository target `make sentinel-herdr-probe-fixture` exercises the recorder
with a representative `pane.agent_status_changed` envelope and runs the
machine-readable checker. The expected alpha result is a valid raw envelope
with probe-local observation time, while host event identity and host event
timestamp remain reported as absent. This is an intentional contract finding,
not a substitute identity or timestamp.

The structured snapshot in
[`herdr-sentinel/plugin/host-contract-status.json`](../../herdr-sentinel/plugin/host-contract-status.json)
records the same boundary as observed, missing, unsupported, or unobserved
fields. The sanitized live fixture under
[`herdr-sentinel/plugin/fixtures/live-pane-agent-status-changed/`](../../herdr-sentinel/plugin/fixtures/live-pane-agent-status-changed/)
keeps the callback shape reviewable without retaining local workspace or
filesystem identifiers.

Sentinel now exposes the raw boundary through
`sentinel adapter herdr-host-envelope --event <path>`. Its Go loader preserves
the raw envelope for inspection, while the existing normalized
`ingen.herdr-event/v1` loader rejects the same input before any receipt update.
The repository target `make sentinel-herdr-host-envelope-check` covers the
accepted evidence path.

The live probe was linked and enabled in the local Herdr session. Herdr's
plugin log recorded successful invocations for real `pane.agent_status_changed`
callbacks. The recorder now prints each capture directory into that log so the
host-selected state path can be inspected even when the runtime cleans up its
temporary state after the callback.

## Sources

- [Herdr plugin documentation](https://herdr.dev/docs/plugins/)
- [Herdr Socket API](https://herdr.dev/docs/socket-api/)
- [Herdr CLI reference](https://herdr.dev/docs/cli-reference/)
- [`sentinel-herdr-host-binding-contract.md`](sentinel-herdr-host-binding-contract.md)
