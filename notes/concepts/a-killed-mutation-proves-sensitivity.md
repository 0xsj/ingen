# A killed mutation proves sensitivity, not correctness

A clean baseline plus a failed run against a deliberate behavioral defect shows that an oracle can detect that change, not that the entire subject is correct.

## Origin

The first document-pipeline defect changes successful creation from HTTP 202 to
HTTP 200. The contract's valid-create rule should fail against that variant.

## What

A mutation campaign needs two separate observations: the clean subject satisfies
the contract, and a named mutated subject violates an intended rule. The
mutation is killed when at least one declared target rule fails directly. The
run must retain the mutation identity and the affected rule rather than
reporting only an aggregate score.

## Why

A passing baseline alone can be compatible with a weak oracle. A mutation score
without a clean baseline can also hide an already-broken subject. Keeping the
two runs distinct makes sensitivity evidence legible and prevents “killed” from
being mistaken for a proof of general correctness.

## Example

```text
clean subject:       document.create.valid.accepted -> pass
status-200-create:   document.create.valid.accepted -> fail
```

## Gotchas

- A defect that also prevents setup may make a stateful rule `inconclusive`,
  which is different from killing the target assertion.
- Equivalent or unobservable mutations must not be counted as killed.
- The defect description must say what behavior changed and which rule should
  observe it before the mutated run begins.

## Used in

- [`status-200-create defect`](../../examples/document-pipeline-lab/defects/status-200-create/)
- [`document-pipeline contract`](../../examples/document-pipeline-lab/contract/contract.yaml)
- [`sorna/internal/mutation`](../../sorna/internal/mutation/)
- the future Sorna mutation campaign

## Related

- [`Sorna mutation specification`](../../sorna/MUTATION-SPEC.md)
- [`The first Sorna runner accepts a subject URL instead of a subject package`](../modules/sorna-http-runner.md)
