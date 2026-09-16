# A failed Sentinel receipt must remain non-passing at the Nublar boundary

## Claim

Sentinel owns lifecycle meaning, but its shared handoff must make failure
unambiguous. Nublar must preserve that non-passing decision without reading
Sentinel's producer-owned report.

## What

`sentinel run ci-result` maps a terminal `failed` receipt to a shared
`ingen.ci-result/v1` envelope with `status: failed` and `exit_code: 1`.
Blocked or incomplete lifecycle states map to `status: error` and `exit_code:
2`, with the lifecycle reason in the envelope error field. Nublar's
`sentinel-webhook-verifier` workflow consumes that envelope like any other
producer result; its coordinator status is therefore failed or error, never
passed.

## Why

The receipt is not a behavioral verdict, and Nublar must not infer one from
Sentinel's nested report. The shared status and exit-code contract is the
smallest reliable signal for downstream CI: Sentinel decides how its lifecycle
ended, while Nublar composes that decision and preserves the opaque evidence.

## Proof

- `herdr-sentinel/cmd/sentinel/main_test.go` verifies a failed receipt is
  published as a failed envelope even though the command returns producer exit
  code `1`.
- `nublar/integration/mixed_producers_test.go` verifies Nublar collects the
  failed Sentinel envelope as a failed run and retains the exact producer
  report bytes.
- The same integration boundary verifies a blocked Sentinel envelope becomes
  a Nublar `error` run with exit code `2`.
- `herdr-sentinel/cmd/sentinel/main_test.go` verifies the blocked lifecycle
  mapping and preserves its reason in the shared envelope.

The host-enabled expected-failure target is
`make nublar-sentinel-run-collect-failure-fresh`. It uses the controlled webhook
duplicate-idempotency defect, allows the expected Sentinel/Sorna producer
failure to continue to envelope emission, and asserts that Nublar records a
failed run. The proof completed on 2026-09-16 in
`/private/tmp/ingen-sentinel-failure-workspace.aoumsG`: Sorna reported three
passed rules and one failed rule, Sentinel emitted `failed/1` with
`audit_status: passed`, and Nublar stored `failed/1`.
- The surrounding Sentinel and Nublar package tests pass with the boundary
  regression included.

## Limits

This is a local envelope and collection proof. It does not establish native
Herdr callback attestation, hosted retries, or remote retention. Those remain
separate integration boundaries.

## Related

- [Sentinel verifier receipts enter Nublar through the shared CI envelope](nublar-sentinel-verifier-workflow.md)
- [A nested CI explanation needs its own closed contract](sentinel-ci-explanation-contract.md)
- [Nublar's decision must stay at the shared envelope boundary](nublar-envelope-coordinator.md)
