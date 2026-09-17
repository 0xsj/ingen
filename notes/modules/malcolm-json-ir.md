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
fixed array order. It may also carry typed mutation declarations, a narrow
provenance section, and a narrow scenario-isolation reset declaration; these
are explicit handoff data rather than evaluator logic. Mutations are lowered
into a separate Sorna mutation catalogue, while the first provenance
requirement is lowered into target-rule expectations and isolation is copied
into each rule's executable `given` data.
Fixture declarations remain separate specification-level metadata: the IR
preserves their oracle ownership and digest identity without embedding bytes.

## Why

The IR is a boundary between Malcolm and future consumers. Giving it a schema
tag and explicit fields makes version negotiation and cross-language testing
possible. Running validation before mapping prevents an incomplete AST from
being turned into a misleading artifact. Keeping expressions as strings
preserves the current language boundary: Sorna can receive the contract shape
before Malcolm has committed to an expression evaluator. Typed request
literals are the deliberate exception because Sorna must execute their JSON
values rather than interpret prose. Mutation declarations are another typed
exception: Sorna needs their stable target and expected rule binding to plan a
mutation campaign, but still owns mutation execution and evidence. Provenance
declarations are typed intent only; Amber owns the value semantics, while
Sorna observes the declared public response signal.

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
- An IR containing mutations must be passed to the Sorna bridge with an
  explicit mutation-catalogue output path; the contract output alone is not
  the complete handoff.
- The first provenance requirement is intentionally limited to a non-empty
  execution ID in the public Amber-Provenance response header. It is not a
  complete Amber v1 validator or an automatic context injector.
- The first isolation declaration is intentionally limited to one subject-owned
  `POST` reset request before each scenario case. It records the requested
  boundary but cannot prove that private subject resources were cleared.
- Fixture declarations currently carry only an oracle-owned ID, purpose, and
  SHA-256 digest. A separate Sorna fixture-provider handoff can verify a
  relative provider file against that digest, but the IR still does not load
  files or materialize request data.

## Used in

- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/README.md`](../../../malcolm/README.md)

## Related

- [`Malcolm validates completeness after parsing`](malcolm-semantic-validation.md)
- [`The contract can cross language boundaries`](../concepts/the-contract-can-cross-language-boundaries.md)
- [`A passing baseline is not evidence of a useful oracle`](sorna-baseline-precondition.md)
- [`Notes protocol`](../../../NOTES.md)
