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
rule ID under its scenario. The adapter currently lowers status equality,
top-level response-field presence, and top-level response-field equality.

The sorna-malcolm command is only a file-format adapter. It does not run the
subject, freeze an oracle, or reinterpret Sorna results. After translation,
Sorna's existing contract validator remains the authority for the target
contract shape.

## Why

Malcolm's first given clauses are free-form text, and its must_not clauses do
not yet have a corresponding positive expectation in Sorna's current HTTP
runner. Silently dropping either would produce an artifact that looks
complete while asserting less than the source. The adapter therefore fails
with an explicit message until a reviewed lowering exists.

The healthcheck example has no request body and uses only lowerable assertions,
so it provides a real bridge proof without inventing request-data semantics.
The generated contract is still draft: sealing, oracle freezing, and subject
execution remain Sorna operations.

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
  request body.
- must_not and event emission are not lowered by this slice. Rejection is
  evidence of an honest boundary, not a feature gap to hide.
- Passing Sorna contract validation proves structural compatibility, not that a
  running subject satisfies the resulting contract.

## Used in

- sorna/internal/malcolm
- sorna/cmd/sorna-malcolm
- malcolm/examples/healthz.malcolm
- Makefile target malcolm-sorna-contract
- sorna/README.md

## Related

- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Sorna contract specification](../../../sorna/CONTRACT-SPEC.md)
- [Notes protocol](../../../NOTES.md)
