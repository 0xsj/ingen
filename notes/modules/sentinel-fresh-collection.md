# A Sentinel integration proof must start with a fresh artifact root

## Claim

A passing Sentinel-to-Nublar collection is stronger evidence when no prior
receipt, oracle, or verifier bundle can satisfy the workflow accidentally.

## Origin

The ordinary `nublar-sentinel-run-collect` target is useful for the fast local
loop, but it writes under the repository's configured artifact root. That is
appropriate for inspection and iteration, yet it leaves open a stale-output
question when the target is used as an integration proof.

## What changed

`nublar-sentinel-run-collect-fresh` copies the source tree to a temporary
workspace while excluding `.git`, `.artifacts`, and `.cache`. It reuses the
validated Go caches, then runs the existing host-enabled Sentinel→Sorna→Nublar
target inside that workspace with `ARTIFACT_ROOT=.artifacts` pinned locally.
The temporary workspace path, artifact root, and Nublar run store are printed
when the target exits so the proof remains inspectable, even if the caller had
configured a different artifact root in the source workspace.

## Why

The fresh target tests the actual bootstrap, oracle freeze, verifier handoff,
audit-gated CI explanation, and Nublar collection sequence from an empty
artifact root. It does not claim remote retention, queue durability, or native
Herdr plugin execution; those remain separate boundaries.

## Proof

The host-enabled proof completed on 2026-09-16 in
`/private/tmp/ingen-sentinel-workspace.RYOpy3`. It produced a passed
`ingen.nublar-run/v1` result, with Sorna reporting four passed rules and the
preserved Sentinel explanation reporting `audit_status: passed`. The temporary
workspace remains available for inspection.

## Gotchas

- The target requires the host-enabled Sorna path, including its macOS
  Seatbelt/process-inspection permissions.
- The source tree must compile before the host workflow can start; unrelated
  dirty or untracked source changes can fail the proof before Sorna runs.
- The temporary copy is intentionally not deleted automatically, so a failed
  proof can be inspected. The printed path is the operator's cleanup handle.
- Go build and module caches are reused outside the temporary workspace only
  for speed; they are not workflow evidence.

## Used in

- `Makefile` (`nublar-sentinel-run-collect-fresh`)
- `herdr-sentinel/README.md`
- `notes/modules/nublar-sentinel-verifier-workflow.md`

## Related

- [Sentinel verifier receipts enter Nublar through the shared CI envelope](nublar-sentinel-verifier-workflow.md)
- [Audit and emission must share one receipt snapshot](sentinel-receipt-snapshot.md)
- [A nested CI explanation needs its own closed contract](sentinel-ci-explanation-contract.md)
