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

The `plugin/` directory remains a placeholder for the future Herdr integration.
