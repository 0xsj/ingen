# A capability plan makes isolation declarations adapter-ready without claiming enforcement

Sentinel needs to turn a workspace manifest into a concrete handoff for a host
or sandbox adapter. That handoff must remain explicit about what it does not
prove.

## Origin

The workspace manifest declared role read, write, and deny roots, but those
lists were still only embedded in YAML. A future Herdr or host adapter would
otherwise have to reinterpret them independently.

## What

`ingen.sentinel-capability-plan/v1` is a derived JSON artifact. It preserves
the workspace manifest identity, implementation roots, and normalized role
capabilities. The compiler validates that no role both allows and denies an
overlapping path, and that the oracle writer denies every implementation root
without allowing one through a read or write root.

## Why

This is a real boundary between declaration and enforcement. Sentinel can
prepare one reviewable plan while a platform-specific adapter later maps it to
macOS Seatbelt, containers, or another capability mechanism. Sorna remains the
authority for its own subject and oracle policies; Sentinel does not translate
those policies into behavioral rules.

## Gotchas

- `declaration-only` and `unverified` are intentional output fields; compiling
  a plan does not restrict a process.
- Path overlap includes parent/child relationships, not only exact string
  equality. An allowed `.` would therefore conflict with a denied `.git`.
- The oracle rule is checked against the workspace's top-level
  `implementation_roots`, including defect and mutation-source roots that may
  not be writable by the implementation role.
- Workspace manifests and policy references are resolved under the current
  project root when the plan is created; symlink escapes are rejected before
  their bytes are hashed into the plan.
- Host-specific policy syntax, process launch, access telemetry, and signed
  attestations remain outside this slice.

## Result

```sh
make sentinel-capability-plan
```

This produces `.artifacts/sentinel-webhook-capability-plan.json` with the
status explicitly marked `declaration-only` / `unverified`.

## Used in

- `herdr-sentinel/internal/capability`
- `herdr-sentinel/cmd/sentinel`
- `herdr-sentinel/spec/capability-plan-v1.schema.json`
- `make sentinel-capability-plan`

## Related

- [`sentinel-contract-workspace.md`](sentinel-contract-workspace.md)
- [`sentinel-run-receipt.md`](sentinel-run-receipt.md)
- [`sorna-sandbox-enforcement.md`](sorna-sandbox-enforcement.md)
- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
