# A nested CI explanation needs its own closed contract

The shared CI envelope can keep Sentinel meaning opaque only if Sentinel still defines the shape of that opaque explanation explicitly.

## Origin

The Sentinel CI explanation began as an inline Go struct and then gained an audit trace. Without a versioned schema, a downstream consumer could not validate the audit context without importing Sentinel code or accepting silently changed fields.

## What

`ingen.sentinel-ci-explanation/v1` defines the required lifecycle outcome and artifact IDs plus the optional audit status and compact check statuses. The schema rejects unknown fields and constrains statuses while the outer `ingen.ci-result/v1` envelope continues to treat the explanation as producer-owned JSON.

## Why

The losing alternatives were an undocumented nested object and moving Sentinel fields into the shared envelope. The first makes the handoff drift-prone; the second makes Nublar understand Sentinel semantics. A separate closed schema gives Sentinel evolution and validation without breaking the coordinator’s neutrality.

## Gotchas

- The explanation describes Sentinel lifecycle and integrity context; it is not a Sorna result schema.
- `audit_status` is optional because direct library construction can emit the legacy receipt-only explanation; the CLI CI path includes it after the audit gate.
- Schema validation defines structure, not whether the referenced files still exist after emission; input hashes remain the provenance boundary.

## Used in

- [`herdr-sentinel/internal/run/ciresult.go`](../../herdr-sentinel/internal/run/ciresult.go)
- [`herdr-sentinel/spec/ci-explanation-v1.schema.json`](../../herdr-sentinel/spec/ci-explanation-v1.schema.json)
- [`nublar-sentinel-verifier-workflow.md`](nublar-sentinel-verifier-workflow.md)

## Related

- [`ci-result-exit-code-contract.md`](ci-result-exit-code-contract.md)
- [`sentinel-receipt-snapshot.md`](sentinel-receipt-snapshot.md)
- [`nublar-envelope-coordinator.md`](nublar-envelope-coordinator.md)
