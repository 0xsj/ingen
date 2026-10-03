# Herdr Sentinel

Herdr Sentinel is InGen's interactive workflow surface for Herdr. It creates
the contract workspace, coordinates agent roles and worktrees, applies or
requests capability policies, invokes Sorna, and makes lifecycle and evidence
status visible.

The contract workspace belongs here as a user experience. Sorna consumes the
sealed, canonical contract snapshot and remains the authority for verification
semantics and evidence production.

For a new project, Sentinel can create the initial local namespace with:

```sh
go run ./herdr-sentinel/cmd/sentinel project init --root /path/to/project --id my-project
```

After bootstrapping a receipt, the local process provider can launch one role:

```sh
go run ./herdr-sentinel/cmd/sentinel session spawn \
  --workspace .ingen/workspace.yaml \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root /path/to/project --role contract-author -- your-agent-command
```

This captures session identity and output but reports `declaration-only`
enforcement. It is a local workflow provider, not yet a sandbox or native
Herdr session binding.

Before launching roles, run the project preflight:

```sh
go run ./herdr-sentinel/cmd/sentinel project check \
  --root /path/to/project
```

It checks the workspace, capability plan, Sorna policies, Nublar workflow, and
contract state. A fresh scaffold reports `incomplete` until a contract exists
and the host enforcement boundary is wired. Structural errors report
`blocked`; use `--format json` for an agent-readable result.

From the InGen checkout, the same preflight and contract creation paths are
available through Make:

```sh
make sentinel-project-check SENTINEL_PROJECT_ROOT=/path/to/project
make sentinel-project-contract-create SENTINEL_PROJECT_ROOT=/path/to/project
make sentinel-project-contract-seal SENTINEL_PROJECT_ROOT=/path/to/project
make sentinel-project-oracle-freeze SENTINEL_PROJECT_ROOT=/path/to/project
```

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

When the project is outside the caller's working directory, bootstrap accepts
the project root explicitly and keeps the recorded workspace path relative to
it:

```sh
go run ./herdr-sentinel/cmd/sentinel run bootstrap \
  --workspace workspaces/webhook-validation.yaml --root /path/to/project \
  --output /path/to/project/.artifacts/sentinel-webhook-run.json
```

Compile the declaration-only capability handoff with:

```sh
make sentinel-capability-plan
```

This checks that allowed and denied roots do not overlap and that the oracle
writer denies every declared implementation root. It is ready for a future
host adapter, but it is not itself an enforcement mechanism.

The Sentinel contract commands are thin entry points over Sorna's public
contract package, so the contract rules and hashes have one owner:

```sh
go run ./herdr-sentinel/cmd/sentinel contract create \
  --root /path/to/project --ingen-root /path/to/ingen
go run ./herdr-sentinel/cmd/sentinel contract validate \
  .ingen/contract/contract.json --root /path/to/project
go run ./herdr-sentinel/cmd/sentinel contract seal \
  .ingen/contract/contract.json --root /path/to/project \
  --output-dir .ingen/contract
```

The independent oracle and black-box verification path is also available from
Sentinel without a native Herdr session:

```sh
go run ./herdr-sentinel/cmd/sentinel oracle freeze \
  --root /path/to/project --ingen-root /path/to/ingen
go run ./herdr-sentinel/cmd/sentinel verify \
  --root /path/to/project --ingen-root /path/to/ingen \
  --subject-command /path/to/project/bin/subject \
  --subject-arg=-addr --subject-arg 127.0.0.1:8080
go run ./sorna/cmd/sorna evidence verify \
  /path/to/project/.ingen/artifacts/evidence
go run ./sorna/cmd/sorna gate \
  /path/to/project/.ingen/artifacts/evidence
```

`oracle freeze` must run before implementation work. `verify` starts the
subject under Sorna's managed-subject policy and evaluates it through the
public boundary; it does not ask the subject to read the contract or oracle.
The generated project policy binds the project ID to the Malcolm contract ID,
so those IDs must match.

When the project is outside the caller's working directory, load the manifest
and its policy references from that project root explicitly:

```sh
go run ./herdr-sentinel/cmd/sentinel workspace capabilities \
  --workspace workspaces/webhook-validation.yaml --root /path/to/project
```

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

For the first manual fresh-project integration path, see
[`FRESH-PROJECT-GUIDE.md`](FRESH-PROJECT-GUIDE.md). The reusable starting
workspace declaration is
[`workspaces/fresh-project.yaml`](workspaces/fresh-project.yaml).

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
  --root . \
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

The `plugin/` directory contains the version-pinned Herdr compatibility probe
and remains the place for the production binding. The probe targets Herdr 0.9.0
and captures raw event hooks without appending to Sentinel receipts; see its
[`README.md`](plugin/README.md) and the
[`Herdr 0.9.0 compatibility note`](../notes/modules/sentinel-herdr-v0-9-compatibility.md).
The checked-in contract snapshot and sanitized live fixture are also available
under [`plugin/host-contract-status.json`](plugin/host-contract-status.json)
and [`plugin/fixtures/`](plugin/fixtures/).

The raw callback inspection seam is available with
`sentinel adapter herdr-host-envelope --event <path>`; it is evidence-only and
does not update a Sentinel receipt.

Inspect recorded local role sessions without opening artifact files directly:

```sh
go run ./herdr-sentinel/cmd/sentinel session list \
  --root /path/to/project
go run ./herdr-sentinel/cmd/sentinel session status \
  --root /path/to/project --path .ingen/artifacts/sessions/<session-id>.json
```

After the required roles and producer artifacts are present, close the
receipt before auditing and handing it to Nublar:

```sh
go run ./herdr-sentinel/cmd/sentinel run status \
  --receipt .ingen/artifacts/sentinel-run.json \
  --root /path/to/project --status completed
```

The same command can move a terminal receipt to `cleaned` after its artifacts
are no longer needed locally.
