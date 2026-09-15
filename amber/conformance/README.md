# Conformance fixtures

[`v1.json`](v1.json) contains shared valid and invalid v1 wire values plus
deterministic `Child`, `Retry`, and `Replay` transition cases.
[`http-v1.json`](http-v1.json) and [`messaging-v1.json`](messaging-v1.json)
cover the transport envelopes. IDs are fixed only so implementations can
compare decoded values; newly created executions MUST still generate fresh IDs
according to the specification.
[`logging-v1.json`](logging-v1.json) covers the default structured-log field
projection, and [`tracing-v1.json`](tracing-v1.json) covers the matching
framework-neutral tracing attribute projection. [`storage-v1.json`](storage-v1.json)
covers idempotent writes, conflict detection, missing-record behavior, logical
work history, immediate causation lookup, and correlation history.
[`otel-v1.json`](otel-v1.json) covers the OpenTelemetry-compatible form of the
same stable `amber.*` tracing attribute projection.

Conformance runners SHOULD:

1. decode every value in `valid` and accept it;
2. decode every value in `invalid` and reject it; and
3. run every case in `incoming` with its declared policy and expected presence;
4. run every case in `transitions` with its `generated_ids` sequence and
   compare the resulting semantic value with `expected`; and
5. compare the semantic fields of equivalent values, ignoring JSON whitespace
   and object-key order.

The transition fixture uses `input` names from the `valid` cases. Implementations
may inject the listed IDs only in tests or controlled tooling; production
defaults must continue to use secure random UUIDv4 generation.
