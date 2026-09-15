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

The `plugin/` directory remains a placeholder for the future Herdr integration.
