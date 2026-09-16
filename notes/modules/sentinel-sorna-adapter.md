# Sentinel should delegate enforcement instead of duplicating Sorna policy semantics

Sentinel needs a real execution seam, but it should not become a second
Seatbelt or policy interpreter.

## Origin

The capability plan described role roots and marked itself
`declaration-only`. Sorna already owns the macOS Seatbelt backend and the
policy semantics, so Sentinel needed a narrow way to select the correct policy
and hand off execution.

## What

The capability plan now binds exact SHA-256 references for the workspace
manifest, oracle policy, and subject policy. The first adapter prepares an
oracle-writer command for Sorna's existing CLI:

```sh
sentinel adapter oracle --workspace <path> [--root <dir>] -- <command> [args...]
```

Before delegation, Sentinel validates the plan and rechecks the workspace and
oracle-policy bytes against the plan references, copies the verified policy to
a temporary read-only snapshot, and invokes `sorna sandbox exec --policy ...`
against that snapshot. Policy loading, Seatbelt profile generation, process
restrictions, and enforcement evidence remain Sorna responsibilities. The
workspace, policy, and frozen-oracle references are resolved under the supplied
root with symlink escapes rejected before a handoff is prepared.

The verifier adapter composes Sorna's managed `run` command separately. It
binds a frozen oracle plus distinct oracle and subject-policy snapshots, then
passes subject lifecycle arguments through without reinterpreting them.
Before launch, its subject root must resolve within the supplied project root;
an escaping or unresolved subject root fails preparation.

When given a Sentinel receipt, the adapter records the policy handoff and
Sorna start before launch, then records Sorna completion and the process
outcome after launch.

## Why

This is a genuine product boundary. Sentinel owns role selection and
orchestration; Sorna owns the meaning and enforcement of its capability policy.
The adapter is therefore small and explicit about the handoff instead of
reimplementing filesystem, network, or process rules.

## Findings

- A prepared command is not an enforcement result. The adapter reports
  `pending-host-enforcement` until Sorna actually runs.
- The temporary snapshot closes the ordinary replacement window between
  planning and Sorna's policy load, but it is not a signed or externally
  attested artifact. A higher-assurance adapter still needs an immutable
  trusted handoff.
- The oracle and verifier adapters now have separate handoffs. The
  mutation-runner still needs explicit policy composition rather than an
  accidental reuse of either adapter's policy.
- The command currently uses `go run ./sorna/cmd/sorna` as the local Sorna
  entry point; a packaged Sorna binary or Herdr-managed process should replace
  that default in a later integration.

## Result

The adapter can be exercised on macOS with:

```sh
go run ./herdr-sentinel/cmd/sentinel adapter oracle \
  --workspace herdr-sentinel/workspaces/webhook-validation.yaml \
  --root . --receipt .artifacts/sentinel-webhook-run.json \
  -- /bin/cat examples/webhook-validation-lab/contract/contract.yaml
```

The command is intentionally platform-sensitive because actual enforcement is
delegated to Sorna's Seatbelt backend.

## Used in

- `herdr-sentinel/internal/adapter`
- `herdr-sentinel/cmd/sentinel`
- `herdr-sentinel/internal/capability`
- `sorna/cmd/sorna sandbox exec`

## Related

- [`sentinel-capability-plan.md`](sentinel-capability-plan.md)
- [`sorna-sandbox-enforcement.md`](sorna-sandbox-enforcement.md)
- [`sorna-policy-definition.md`](sorna-policy-definition.md)
- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
