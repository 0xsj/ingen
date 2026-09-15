# A second vertical should enter Nublar through the same opaque CI envelope

Nublar can support another InGen vertical without learning that vertical's
contract, mutation, or provider semantics.

## Origin

The webhook source-provider campaign became a strict Sorna path, creating the
first opportunity to test whether Nublar's workflow boundary was genuinely
cross-vertical.

## What

The webhook workflow declares four required Sorna envelopes: provider review,
provider preparation, behavioral verification, and mutation campaign. Nublar
resolves those paths under the artifact root, validates the shared envelopes,
preserves their complete producer reports, and composes only their statuses.

## Why

The workflow belongs to the delivery surface, while contract evaluation and
mutation meaning belong to Sorna. A second workflow file is therefore useful
evidence of a real boundary: the coordinator changes its declared inputs, not
its interpretation of webhook behavior.

## Gotchas

- Workflow result paths are relative to the artifact root, not the repository
  root.
- Every required producer envelope must be generated before aggregation; a
  missing result is a Nublar collection error rather than a Sorna finding.
- The workflow records exact file identities, but does not yet semantically
  seal workflow meaning.

## Result

The fresh webhook workflow passed with four preserved Sorna result artifacts.
The aggregate and persisted run both returned `passed` with exit code `0`.
Measured identities from the fresh workspace were:

- workflow: `365fc7086be55db02303ac808f103e976ed4eb50dc824dc0e8f9084bd7df8d4b`;
- aggregate: `bd289d981c8e41ab0f9d5f6f6e6b2fde8bd596ed6d3355ea293ff9d9650e6d0e`;
- persisted run: `e6251dd23795831a42dcd890e9f0cd90b9dca8a59ef40cf4656e7de392abedab`.

The aggregate is run by `make nublar-webhook-aggregate`; the persisted run
form is available through `make nublar-webhook-run-collect`.

## Used in

- `nublar/workflows/webhook-validation.yaml`
- `nublar aggregate --workflow`
- `nublar run collect --workflow`
- `make nublar-webhook-aggregate`

## Related

- [`nublar-workflow-declaration.md`](nublar-workflow-declaration.md)
- [`nublar-envelope-coordinator.md`](nublar-envelope-coordinator.md)
- [`webhook-go-source-provider.md`](webhook-go-source-provider.md)
