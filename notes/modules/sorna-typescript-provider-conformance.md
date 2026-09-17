# TypeScript can target the frozen Sorna provider handoff

The first cross-language probe uses a deliberately narrow TypeScript adapter
at [`sorna/providers/typescript/`](../../sorna/providers/typescript/). It reads
the exact bytes of a ready mutation plan, derives one exact capability tuple
per mutation shape, creates one prepared entry per mutation, and binds the
manifest to the exact plan SHA-256.

The adapter does not mutate TypeScript source, build a subject, launch a
process, or classify behavioral evidence. That scope is intentional: the
question for this slice is whether another SDK can target the frozen
`ingen.mutation-provider/v1` boundary without adding language-specific concepts
to Sorna.

The acceptance command runs three TypeScript tests, emits a temporary JSON
manifest, validates that manifest with Sorna's existing Go loader, and performs
a no-execution provider review with mandatory exact plan binding. It also emits
the shared provider-review CI envelope and passes it through Nublar's aggregate
boundary, where the producer report must remain preserved:

```sh
make mutation-typescript-provider-conformance
```

The same target then persists the envelope through `nublar run collect`, reads
it back with `run show`, and emits `ingen.nublar-decision/v1`. The stored run
keeps the Sorna provider-review artifact, while the decision projection carries
only the provider-neutral status and result reference.

The result supports portability of the handoff, not readiness of a second
behavioral mutation provider. The Go source provider remains the alpha
behavioral path. A future TypeScript provider should only grow from this probe
if it adds a concrete mutation-preparation or campaign proof.
