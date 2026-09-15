# The alpha boundary should be named before it is expanded

An executable slice becomes easier to reason about when its cross-tool
artifacts and non-guarantees are named before new operators, providers, or
products are added.

## Origin

The document-pipeline workflow now completes a strict four-check path: clean
behavioral verification, provider review, provider preparation, and mutation
campaign verification. The next risk was expanding the implementation without
being explicit about which parts another tool could safely consume.

## What

`ALPHA-INTERFACES.md` is a checkpoint, not a promise that the current design is
finished. It lists the versioned artifacts at the Sorna/Nublar boundary,
records the invariants that currently connect them, and separates those from
internal evidence and unresolved product surfaces.

`make alpha-interface-check` makes the checkpoint repeatable without requiring
generated artifacts. `make nublar-aggregate-fresh` remains the aggregate
end-to-end workflow proof, while `make nublar-run-collect-fresh` additionally
proves the persisted run path.

## Why

Without a named boundary, every successful experiment can quietly become a
new dependency. That creates an artificial module split in one direction and
an accidental compatibility promise in the other. The checkpoint lets us add
depth where the current semantics need it while keeping future Sentinel,
Nublar, and SDK work from importing Sorna internals by convenience.

## Gotchas

- A `v1` schema in this repository is an alpha interface, not a claim of
  production compatibility forever.
- Hashes bind exact bytes and detect drift; they do not prove correctness,
  isolation, or non-observation by an agent.
- Nublar composes envelope severity. It must not reinterpret whether a
  mutation was meaningful or whether an oracle is complete.
- The full repository test remains blocked by unrelated Paddock compile
  errors, so the focused check is intentionally scoped to the alpha surfaces.

## Used in

- `ALPHA-INTERFACES.md`
- `Makefile` target `alpha-interface-check`
- The Sorna document-pipeline workflow and Nublar aggregate/run paths

## Related

- [A coordinator should compose verdicts, not reinterpret findings](nublar-envelope-coordinator.md)
- [Provider preparation can cross the CI boundary before execution](sorna-preparation-ci-result.md)
- [A killed mutation proves sensitivity, not correctness](../concepts/a-killed-mutation-proves-sensitivity.md)
