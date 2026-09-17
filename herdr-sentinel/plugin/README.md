# Herdr Sentinel compatibility probe

This directory contains a deliberately non-production Herdr plugin for
inspecting the host event surface available in Herdr 0.9.0. It captures the
raw event envelope and injected workspace/tab/pane/plugin context under
`HERDR_PLUGIN_STATE_DIR/events/`.

The probe does not append Sentinel events, derive event identity, or claim
delivery acknowledgement. It is intended to answer the host-contract questions
before the production binding is implemented.

Link it locally with:

```sh
herdr plugin link /path/to/ingen/herdr-sentinel/plugin
herdr plugin enable ingen.herdr-sentinel.probe
```

Inspect the configured state directory with:

```sh
herdr plugin config-dir ingen.herdr-sentinel.probe
herdr plugin list --json
```

After exercising a workspace, inspect the plugin state directory reported by
the host. Each `capture.*` directory contains the host event name, raw JSON
event payload, invocation context, captured-at time, and available IDs.
The recorder also prints the capture directory to Herdr's plugin command log,
which makes ephemeral host state locations observable during a live probe.

The captured-at value is probe-local observation time, not a host event
timestamp. The probe intentionally leaves that distinction visible.

For a machine-readable compatibility result after capturing events, run:

```sh
sh /path/to/ingen/herdr-sentinel/plugin/verify-captures.sh \
  /path/to/herdr/plugin/state
```

The checker validates the raw envelopes and reports whether host event identity
and host timestamps are actually present. Missing identity or timestamps leave
the result explicitly blocked for Sentinel production binding.

The checked-in [`host-contract-status.json`](host-contract-status.json) records
the current observed, missing, and unobserved host fields. A sanitized live
envelope is preserved under
[`fixtures/live-pane-agent-status-changed/`](fixtures/live-pane-agent-status-changed/).
