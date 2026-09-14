# A mutation catalogue makes the experiment reviewable before execution

Sorna now has a small versioned catalogue format for declaring mutation
experiments before a subject process is launched. The catalogue gives each
mutation a stable ID, plane, operator, target, human description, reproducible
change, expected contract rule IDs, and lifecycle status.

## Why this boundary matters

Mutation execution needs provider-specific mechanics, but the experiment's
meaning should remain inspectable and language-neutral. A catalogue lets a
reviewer ask what is being changed and what should detect it before any
language SDK or source-level mutator runs.

The `contract_id` and `contract_version` bind the catalogue to its intended
subject contract. `sorna mutation validate --contract` checks that identity and
that every expected rule exists in the contract. A later campaign runner must
also carry this comparison into the sealed-oracle evidence and, eventually,
bind the exact contract hash.

## What we learned

- Stable IDs are the join key between declaration, execution, classification,
  and evidence.
- `implementation` and `contract` are separate mutation planes; they should
  not be silently collapsed into one score.
- An operator name is metadata, not execution support. A future provider must
  explicitly advertise which operators it can apply.
- Validation is intentionally not mutation execution. This keeps the
  catalogue safe to inspect in CI and prevents a declaration from becoming an
  implicit permission to edit a working tree.
- Expected rule IDs are declared before the run, preserving the causal claim
  that a result was sensitive to the intended observable.

## Commands

```sh
make mutation-catalogue-validate
go run ./sorna/cmd/sorna mutation list examples/document-pipeline-lab/mutations/catalogue.yaml
```

## Limits

The first format supports structured `from`/`to` changes and structural
validation. It remains a declaration rather than an execution instruction;
the Go provider and campaign runner consume it only after plan construction.
The current Go provider supports one document-lab operator, not a general
language SDK or operator pack.

## Used in

- [`sorna/internal/mutation`](../../sorna/internal/mutation/)
- [`document-pipeline catalogue`](../../examples/document-pipeline-lab/mutations/catalogue.yaml)
- [`Sorna mutation specification`](../../sorna/MUTATION-SPEC.md)

## Related

- [`A mutation result needs a passing baseline`](sorna-baseline-precondition.md)
- [`Mutation results separate target sensitivity from setup fallout`](sorna-mutation-result-model.md)
