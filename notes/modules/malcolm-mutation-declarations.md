# Malcolm mutation declarations stay separate from the behavioral contract

Malcolm can declare a small, reviewable implementation mutation without taking
ownership of mutation execution. The declaration travels in `malcolm.ir/v1`
and the Sorna adapter emits it as a separate
`ingen.mutation-catalogue/v1` artifact.

## Origin

The original Malcolm design showed mutation intent next to scenarios, but the
first executable slices only emitted behavioral contract rules. Sorna already
has a versioned mutation catalogue, campaign plan, provider, and evidence
boundary, so the smallest useful implementation is a typed handoff into that
existing owner.

## What

The first supported declaration changes one HTTP response status:

~~~text
mutation "return-200" {
  target submit_document
  change response.status from 202 to 200
  expect rule submit_document.requirement.1 to fail
}
~~~

`target` names an existing scenario. `change` currently supports only
`response.status`, with distinct HTTP status values from 100 through 599.
`expect rule` must identify an existing requirement in the target scenario.
Malcolm generates the stable rule ID in the form
`SCENARIO.requirement.N`, and semantic validation checks the reference before
IR emission.

The IR representation is intentionally typed:

~~~json
{
  "id": "return-200",
  "scenario": "submit_document",
  "change": {
    "field": "response.status",
    "from": 202,
    "to": 200
  },
  "expected_rule": "submit_document.requirement.1"
}
~~~

The bridge lowers this to Sorna's existing catalogue fields:

~~~text
plane: implementation
operator: response.status.replace
target: POST /documents
change: {from: 202, to: 200}
expected_rule_ids: [submit_document.requirement.1]
status: candidate
~~~

## Why

Malcolm describes the intended mutation and binds it to a contract rule.
Sorna owns provider capability, campaign planning, subject execution, baseline
comparison, and killed/survived classification. Keeping the catalogue
separate means the behavioral contract remains a stable oracle input and the
mutation cannot quietly change the contract it is meant to challenge.

The `sorna-malcolm` bridge requires `--mutations-output` when the IR contains
mutation declarations. This makes the separate artifact an explicit handoff
instead of silently dropping valid source intent.

## Gotchas

- This slice declares a mutation; it does not edit source code or launch a
  mutated subject.
- Only response-status replacement is supported. Other fields, event changes,
  and contract-plane mutations remain outside Malcolm's syntax.
- The expected rule must be a target scenario requirement, not a setup
  requirement or an arbitrary string.
- A generated catalogue still needs Sorna's contract binding and provider
  validation before a campaign can run.
- Provenance declarations remain deferred to Amber, and array-index response
  selectors remain unsupported.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`malcolm/examples/document_mutation.malcolm`](../../../malcolm/examples/document_mutation.malcolm)

## Related

- [`Sorna mutation specification`](../../../sorna/MUTATION-SPEC.md)
- [`A killed mutation proves sensitivity, not correctness`](../concepts/a-killed-mutation-proves-sensitivity.md)
- [`The Malcolm event mutation is killed by the frozen oracle`](malcolm-event-mutation.md)
- [`Notes protocol`](../../../NOTES.md)
