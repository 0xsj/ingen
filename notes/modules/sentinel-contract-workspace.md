# A Sentinel workspace assembles handoffs without becoming a second verifier

Sentinel's contract workspace should describe who can see and write which
artifacts, while Sorna remains the authority for behavioral contract meaning
and evidence.

## Origin

The webhook workflow proved the Sorna-to-Nublar boundary, but Sentinel still
had only prose describing how an agent workspace would be assembled.

## What

The `ingen.sentinel-workspace/v1` manifest names the contract source, Sorna
policy inputs, Nublar workflow, implementation roots, and logical role
workspaces. Each role declares read, write, and deny roots. The validator checks
path safety, unique role workspaces, the required oracle-writer role, and the
oracle writer's explicit denial of every implementation root.

## Why

The workspace is a coordination artifact, not a replacement contract syntax.
If Sentinel interpreted webhook rules or mutation outcomes, the system would
have two authorities and the agent-facing layer could silently diverge from
Sorna. Keeping the manifest structural makes it useful for Herdr while leaving
verification semantics behind the existing Sorna boundary.

## Gotchas

- A logical workspace path is not automatically an enforced OS boundary; a
  future Sentinel adapter must turn these declarations into host capabilities.
- Denying only the clean subject is insufficient when mutation fixtures or
  copied source variants also contain implementation details.
- The manifest references Sorna and Nublar artifacts but does not seal or
  reinterpret them.

## Result

The webhook workspace manifest validates with:

```sh
make sentinel-workspace-validate
```

The validator is structural only. It does not launch agents, invoke Sorna, or
claim that the declared paths are independently enforced.

## Used in

- `herdr-sentinel/workspaces/webhook-validation.yaml`
- `herdr-sentinel/cmd/sentinel`
- `herdr-sentinel/spec/workspace-v1.schema.json`
- `make sentinel-workspace-validate`

## Related

- [`PLUGIN-SPEC.md`](../../herdr-sentinel/PLUGIN-SPEC.md)
- [`nublar-webhook-workflow.md`](nublar-webhook-workflow.md)
- [`sorna-subject-isolation.md`](sorna-subject-isolation.md)
