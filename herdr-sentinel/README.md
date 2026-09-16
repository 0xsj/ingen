# Herdr Sentinel

Herdr Sentinel is InGen's interactive workflow surface for Herdr. It creates
the contract workspace, coordinates agent roles and worktrees, applies or
requests capability policies, invokes Sorna, and makes lifecycle and evidence
status visible.

The contract workspace belongs here as a user experience. Sorna consumes the
sealed, canonical contract snapshot and remains the authority for verification
semantics and evidence production.

The first concrete artifact is a versioned workspace manifest under
[`workspaces/`](workspaces/). It describes role roots, capability paths, the
Sorna policy inputs, and the Nublar workflow reference. It is deliberately not
another behavioral contract: Sentinel validates orchestration handoffs while
Sorna validates the contract and produces evidence.

The example names contract-author, oracle-writer, backend-implementer,
verifier, and mutation-runner roles. These are logical workspaces today; a
future Herdr adapter must turn their declarations into actual worktrees and
enforced capabilities.

Validate the example with:

```sh
make sentinel-workspace-validate
```

Create its first lifecycle receipt with:

```sh
make sentinel-run-bootstrap
```

The receipt records the exact workspace-manifest bytes and a first
`workspace-created` event. Later role, policy, Sorna, review, and cleanup
events can refer to hashed artifacts without Sentinel reinterpreting their
contents.

Compile the declaration-only capability handoff with:

```sh
make sentinel-capability-plan
```

This checks that allowed and denied roots do not overlap and that the oracle
writer denies every declared implementation root. It is ready for a future
host adapter, but it is not itself an enforcement mechanism.

The first execution handoff delegates the oracle-writer role to Sorna:

```sh
go run ./herdr-sentinel/cmd/sentinel adapter oracle \
  --workspace herdr-sentinel/workspaces/webhook-validation.yaml \
  --root . --receipt .artifacts/sentinel-webhook-run.json \
  -- /bin/cat examples/webhook-validation-lab/contract/contract.yaml
```

Sentinel selects and rechecks the bound oracle policy; Sorna remains the
authority that interprets and enforces it. This command covers the
oracle-writer role.

The verifier handoff composes the frozen oracle with separate oracle and
subject policies, then delegates the managed subject run to Sorna:

```sh
make sentinel-adapter-verifier-probe
```

This produces the Sorna run bundle under
`.artifacts/sentinel-webhook-verifier` and can attach its `run.json` to the
Sentinel receipt. Sentinel records the handoff and artifact lineage; Sorna
retains responsibility for enforcement and behavioral evidence.

Expose the completed Sentinel lifecycle to Nublar through the shared CI
envelope with:

```sh
go run ./herdr-sentinel/cmd/sentinel run ci-result \
  --receipt .artifacts/sentinel-webhook-run.json \
  --source-root . --output .artifacts/sentinel-webhook-ci-result.json
```

The matching Nublar proof is `make nublar-sentinel-run-collect`.

For a clean artifact-root proof that excludes stale outputs, use
`make nublar-sentinel-run-collect-fresh`.

To prove the failure handoff with the controlled webhook duplicate defect, use:

```sh
make nublar-sentinel-run-collect-failure-fresh
```

This target intentionally exercises one failed verifier rule, preserves the
failed Sentinel envelope, and succeeds only after Nublar records the expected
`failed` run. It requires the host-enabled Sorna path.

The future Herdr event placement and translation boundary is documented in
[`sentinel-herdr-event-adapter-boundary.md`](../notes/modules/sentinel-herdr-event-adapter-boundary.md).

Until Herdr exposes its native plugin hooks, a provider-neutral event fixture
can exercise that translation boundary:

```sh
go run ./herdr-sentinel/cmd/sentinel adapter herdr-event \
  --receipt .artifacts/sentinel-webhook-run.json \
  --event .artifacts/herdr-event.json
```

For a callback stream, use newline-delimited events. The whole batch is
validated before the updated receipt is published:

```sh
go run ./herdr-sentinel/cmd/sentinel adapter herdr-events \
  --receipt .artifacts/sentinel-webhook-run.json \
  --events .artifacts/herdr-events.jsonl
```

Register a produced file before sending an event that references it:

```sh
go run ./herdr-sentinel/cmd/sentinel run artifact \
  --receipt .artifacts/sentinel-webhook-run.json \
  --id verifier-run --role verifier --kind sorna-run \
  --path .artifacts/sentinel-webhook-verifier/run.json
```

The adapter binds the callback to the receipt's run and workspace, preserves
the Herdr event ID and session reference, and is safe to retry with the same
event ID. An event may also carry an explicit `receipt_status`; Sentinel
validates and applies it atomically with the event. It accepts only lifecycle
event types and artifact IDs already understood by Sentinel. With `--root`, it
also verifies referenced artifact bytes before appending the event. This is an
ingress contract for a future native Herdr plugin, not an attestation of the
host application's callback stream.

Event timestamps are monotonic, and terminal receipts cannot regress to a
non-terminal status; cleanup is the explicit terminal-to-`cleaned` exception.

Artifact registration is also retry-safe when the complete artifact identity
matches an existing reference. Reusing an artifact ID for different metadata,
path, or bytes is rejected as a conflict.

Audit a receipt before exposing it as a completed workflow:

```sh
go run ./herdr-sentinel/cmd/sentinel run audit \
  --receipt .artifacts/sentinel-webhook-run.json \
  --root . --output .artifacts/sentinel-webhook-audit.json
```

The audit verifies the workspace and artifact hashes and distinguishes a
terminal, integrity-checked receipt from an incomplete or tampered one. It
does not reinterpret Sorna results or claim independent attestation. The
`run ci-result` command applies the same integrity gate to terminal receipts
before emitting a shared envelope. Audit and operator-report files are
published as complete files, so readers do not observe an in-progress write.

Render the same receipt and audit as a concise operator view:

```sh
go run ./herdr-sentinel/cmd/sentinel run report \
  --receipt .artifacts/sentinel-webhook-run.json \
  --root .
```

The report keeps lifecycle completion, audit integrity, producer-owned Sorna
meaning, events, artifacts, and evidence limitations visibly separate.

The shared CI envelope emitted by `run ci-result` includes the compact audit
status and check statuses in its Sentinel explanation, alongside the exact
receipt snapshot and artifact input references.

A terminal `failed` Sentinel receipt becomes a `failed` shared envelope with
exit code `1`; blocked or otherwise incomplete receipts become `error` with
exit code `2`. Nublar therefore receives an explicit non-passing decision at
the boundary instead of treating an unfinished workflow as success.

The producer-owned explanation shape is versioned in
[`spec/ci-explanation-v1.schema.json`](spec/ci-explanation-v1.schema.json).

The `plugin/` directory remains the place for native Herdr bindings once the
host application's plugin API is available.
