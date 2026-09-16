# A verifier handoff must preserve the oracle and subject policies separately

The verifier is the first Sentinel handoff that exercises the complete Sorna
managed-run boundary. It consumes a frozen oracle, an oracle policy, and a
subject policy. Those inputs have different meanings and must not be merged
into one generic "test policy".

## What changed

`sentinel adapter verifier` now:

1. validates the declaration-only capability plan and selects the `verifier`
   role;
2. reads the frozen oracle and records its exact SHA-256 reference;
3. rechecks the manifest, oracle policy, and subject policy against the hashes
   in the capability plan;
4. writes distinct read-only snapshots for all three inputs;
5. composes Sorna's existing `run` command with the snapshots and subject
   lifecycle arguments; and
6. optionally records those inputs and Sorna's `run.json` in the Sentinel
   lifecycle receipt.

Example:

```sh
sentinel adapter verifier \
  --workspace herdr-sentinel/workspaces/webhook-validation.yaml \
  --oracle .artifacts/webhook-validation-oracle/oracle.json \
  --base-url http://127.0.0.1:8090 \
  --subject-command .artifacts/webhook-validation-subject/webhook-validation \
  --subject-arg=-addr --subject-arg 127.0.0.1:8090 \
  --output-dir .artifacts/sentinel-webhook-verifier \
  --receipt .artifacts/sentinel-webhook-run.json
```

The equivalent repository probe is:

```sh
make sentinel-adapter-verifier-probe
```

## Why the split matters

The oracle is the frozen behavioral reference. The oracle policy limits the
oracle-writer's contract-read phase. The subject policy limits the managed
implementation process. A verifier may consume all three after the oracle is
frozen, but changing one reference must not silently change the meaning of the
others.

Sentinel therefore composes arguments and records provenance. Sorna still
parses policies, applies the host sandbox, starts and stops the subject, waits
for readiness, compares behavior, and writes the evidence bundle.

## Findings and limits

- A read-only snapshot removes the ordinary replacement window between
  Sentinel planning and Sorna loading the inputs, but it is not external
  attestation.
- The Sentinel receipt records the Sorna bundle's `run.json` when it is
  available. Its contents remain opaque to Sentinel.
- The verifier handoff is currently a local Sorna CLI adapter and reports
  `pending-host-enforcement` until Sorna runs. A packaged Sorna binary or
  Herdr-managed process is a later integration concern.
- Mutation execution is intentionally not included in this handoff yet. It
  needs its own policy composition and artifact lineage rather than reusing a
  verifier command by convention.

## Used in

- `herdr-sentinel/internal/adapter`
- `herdr-sentinel/cmd/sentinel`
- `sorna/cmd/sorna run`
- `Makefile` (`sentinel-adapter-verifier-probe`)

## Related

- [`sentinel-sorna-adapter.md`](sentinel-sorna-adapter.md)
- [`sentinel-capability-plan.md`](sentinel-capability-plan.md)
- [`sorna-run-consumes-frozen-oracle.md`](sorna-run-consumes-frozen-oracle.md)
- [`sorna-subject-isolation.md`](sorna-subject-isolation.md)
