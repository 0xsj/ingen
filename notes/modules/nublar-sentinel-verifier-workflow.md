# Sentinel verifier receipts enter Nublar through the shared CI envelope

Nublar should not parse Sentinel's lifecycle events or become a second
orchestration engine. The first Sentinel integration therefore adapts the
validated verifier receipt to the shared `ingen.ci-result/v1` envelope before
Nublar collects it.

## What

`sentinel run ci-result --receipt <path>` reads and validates one exact
`ingen.sentinel-run/v1` receipt snapshot, preserves those bytes as the opaque
producer report, and exposes only the shared status/exit-code fields plus
hashed input references. Before a terminal receipt is emitted, the command
verifies the workspace and artifact references through Sentinel's audit
boundary, so drift cannot become a passing envelope. A `completed` verifier receipt maps to
`passed`; a `failed` receipt maps to `failed`; non-terminal lifecycle states
map to an `error` envelope.

The Nublar workflow declares the generated envelope rather than the raw
Sentinel receipt:

```yaml
schema: ingen.nublar-workflow/v1
id: sentinel-webhook-verifier
checks:
  - id: verifier-lifecycle
    tool: sentinel
    result: sentinel-webhook-ci-result.json
```

This keeps Nublar's ingress unchanged: it still consumes only validated shared
CI envelopes. The envelope's `inputs` retain the exact Sentinel receipt,
workspace manifest, and receipt artifact references for downstream review. Its
producer-owned explanation also carries the compact audit status and check
IDs/statuses that justified the emission.

## Proof target

```sh
make nublar-sentinel-run-collect
```

The target runs the existing Sentinel verifier handoff, emits the shared
Sentinel envelope, and collects it into a durable Nublar run. The matching
aggregate-only target is `make nublar-sentinel-aggregate`.

The host-enabled proof completed on 2026-09-16 with Sorna's macOS Seatbelt
path: the collected Nublar run was `passed`, and its preserved Sentinel
envelope included the `ingen.sentinel-ci-explanation/v1` audit trace.

Repeat the proof from a fresh source/artifact workspace with:

```sh
make nublar-sentinel-run-collect-fresh
```

This excludes prior `.artifacts` and `.cache` contents from the source copy,
so the result cannot be satisfied by a stale Sentinel receipt or verifier
bundle.

The negative handoff is covered without host execution as well: the Sentinel
CLI test confirms a terminal `failed` receipt emits a failed envelope with
exit code `1`, and the Nublar integration test collects that envelope as a
failed `ingen.nublar-run/v1` decision while preserving the opaque receipt
report. Incomplete or blocked Sentinel receipts remain `error` envelopes.

The host-enabled expected-failure proof is available with:

```sh
make nublar-sentinel-run-collect-failure-fresh
```

It runs the controlled duplicate-idempotency defect through the real Sorna
handoff. The target ignores the producer's expected nonzero exits only long
enough to emit the shared envelope, then asserts that Nublar records the final
`failed/1` decision. The proof completed on 2026-09-16 in
`/private/tmp/ingen-sentinel-failure-workspace.r1JTRr`, with one failed Sorna
rule and a passing Sentinel integrity audit.

## Limits

The raw Sentinel receipt remains Sentinel-owned and is not reinterpreted by
Nublar. The audit summary is provenance for Sentinel's integrity gate, not a
behavioral interpretation of Sorna. This is a local positive-path integration; hosted event ingestion,
external correlation, retries, and remote retention remain future CI or
delivery-adapter concerns. A failed or blocked verifier can be converted with
the `sentinel run ci-result` command directly, even when a convenience Make
target stops before later collection steps.

## Used in

- `herdr-sentinel/internal/run/ciresult.go`
- `herdr-sentinel/cmd/sentinel`
- `nublar/workflows/sentinel-webhook.yaml`
- `Makefile` (`sentinel-ci-result`, `nublar-sentinel-run-collect`)

## Related

- [`sentinel-verifier-adapter.md`](sentinel-verifier-adapter.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`nublar-envelope-coordinator.md`](nublar-envelope-coordinator.md)
