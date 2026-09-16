# Malcolm's JSON IR is a transport boundary, not an evaluator

The first intermediate representation should preserve the validated contract shape in portable bytes without pretending that its expressions are executable yet.

## Origin

Malcolm now has a parser and a semantic validator, but those Rust structs are
an in-process representation. The wider InGen ecosystem includes Go tools such
as Sorna and needs a language-neutral handoff before it can consume Malcolm
contracts.

## What

`compile` validates a `Specification` and maps it into an explicit
`IntermediateRepresentation` tagged `malcolm.ir/v1`. The representation
contains the specification metadata, scenarios, free-form `given` text,
optional typed request bodies and stateful setup data, a required `when`
method/path pair, and requirements whose kinds are serialized as `must` or
`must_not`. `to_json` emits deterministic JSON with a fixed field order and
fixed array order.

## Why

The IR is a boundary between Malcolm and future consumers. Giving it a schema
tag and explicit fields makes version negotiation and cross-language testing
possible. Running validation before mapping prevents an incomplete AST from
being turned into a misleading artifact. Keeping expressions as strings
preserves the current language boundary: Sorna can receive the contract shape
before Malcolm has committed to an expression evaluator. Typed request
literals are the deliberate exception because Sorna must execute their JSON
values rather than interpret prose.

The serializer is handwritten because this first IR has only strings, arrays,
objects, and nulls, and the crate currently has no dependencies. That choice is
deliberately temporary: once the schema grows, a standard serialization
library and a separate schema fixture should replace assumptions spread across
manual string-building code.

## Example

```json
{"schema":"malcolm.ir/v1","specification":{"name":"document_api","version":"v1","subject":"documents","scenarios":[{"name":"submit_document","given":["document is valid"],"when":{"method":"POST","path":"/documents"},"requirements":[{"kind":"must","expression":"response.status == 202"}]}]}}
```

This is portable data. It says what the contract contains; it does not invoke
the subject, evaluate the expression, or establish evidence.

## Gotchas

- Stable JSON field order is an implementation policy, not yet a canonical
  artifact specification. Hashing this output should wait for an explicit
  canonicalization rule.
- Array order is preserved because scenario and requirement order may be
  meaningful to diagnostics and later evidence, even when the contract does
  not use order for behavior.
- JSON escaping is required for quotes, backslashes, and control characters;
  raw expressions must never be interpolated directly into JSON.
- The IR's required `when` field depends on validation having succeeded. The
  internal `expect` is an invariant check, not input validation.
- A valid IR is not proof that the contract is correct or that an implementation
  satisfies it.

## Used in

- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/README.md`](../../../malcolm/README.md)

## Related

- [`Malcolm validates completeness after parsing`](malcolm-semantic-validation.md)
- [`The contract can cross language boundaries`](../concepts/the-contract-can-cross-language-boundaries.md)
- [`A passing baseline is not evidence of a useful oracle`](sorna-baseline-precondition.md)
- [`Notes protocol`](../../../NOTES.md)
