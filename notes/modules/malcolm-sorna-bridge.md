# Malcolm reaches Sorna through a rejecting IR adapter

A cross-language contract handoff is safe only when unsupported Malcolm meaning is rejected instead of being converted into a weaker Sorna rule.

## Origin

Malcolm now emits a portable JSON IR, while Sorna already owns the contract
schema, oracle generation, HTTP execution, and evidence semantics. The first
integration needed to prove that these boundaries can meet without moving
Sorna's verifier logic into the Rust compiler.

## What

The Go adapter in sorna/internal/malcolm loads malcolm.ir/v1, validates the
fields it needs, and translates the supported subset into a draft
ingen.contract/v1 document. Each Malcolm requirement becomes a stable Sorna
rule ID under its scenario. The adapter lowers typed top-level request bodies,
stateful setup requests with positive status/body assertions, top-level
response-field presence/equality, and top-level response-field captures.

The sorna-malcolm command is only a file-format adapter. It does not run the
subject, freeze an oracle, or reinterpret Sorna results. After translation,
Sorna's existing contract validator remains the authority for the target
contract shape.

## Why

Malcolm's free-form given clauses do not yet have an executable lowering in
Sorna, and negative setup clauses remain unsupported. Silently dropping either
would produce an artifact that looks complete while asserting less than the
source. The adapter therefore fails with an explicit message until a reviewed
lowering exists.

The healthcheck example provides a small stateless bridge proof. The
document_flow example additionally proves request bodies, setup assertions,
state labels, and captures. The generated contracts are still draft: sealing,
oracle freezing, and subject execution remain Sorna operations.

## Example

~~~text
Malcolm source -> malcolm.ir/v1 JSON -> Sorna draft contract -> Sorna validation
~~~

Run the complete handoff with:

~~~sh
make malcolm-sorna-contract
~~~

The intermediate files default to .artifacts/malcolm-healthz.ir.json and
.artifacts/malcolm-healthz-contract.json.

## Gotchas

- A valid Malcolm IR is not automatically a valid Sorna contract; the two
  schemas answer different questions and have different required metadata.
- Malcolm's version label v1 is lowered to Sorna's numeric version 1 only
  because the adapter explicitly requires the vN form.
- Free-form given text is not request JSON. It must not be guessed into a
  request body; use a typed given body block.
- Request bodies currently contain only non-empty top-level string, integer,
  and boolean fields.
- Stateful setup expectations are merged before lowering so status and body
  assertions are checked by one setup step.
- Target-rule must_not now preserves Sorna's negative strength for the same
  lowerable expressions. `emit "event.name"` lowers to a required event
  membership check; negative setup requirements remain rejected.
- Passing Sorna contract validation proves structural compatibility, not that a
  running subject satisfies the resulting contract.

## Used in

- sorna/internal/malcolm
- sorna/cmd/sorna-malcolm
- malcolm/examples/healthz.malcolm
- malcolm/examples/document_flow.malcolm
- Makefile target malcolm-sorna-contract
- Makefile target malcolm-sorna-flow-contract
- sorna/README.md

## Related

- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [Malcolm carries typed request bodies and stateful setup across the boundary](malcolm-request-body-lowering.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Sorna contract specification](../../../sorna/CONTRACT-SPEC.md)
- [Notes protocol](../../../NOTES.md)
