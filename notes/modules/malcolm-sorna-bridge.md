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
rule ID under its scenario. The adapter lowers typed request bodies, including
recursive objects, arrays, and deterministic repeat generators,
stateful setup requests with positive status/body assertions, top-level
response-field presence/equality, top-level response-field captures, and
string capture interpolation in later request bodies. Response assertions and
captures support dotted object paths with explicit missing and null semantics.
It also lowers the narrow `isolation per scenario` declaration into each
rule's `given.isolation` reset data; Sorna's runner owns invoking that subject
fixture hook and recording its result.
Oracle-owned digest-pinned fixture declarations are lowered to Sorna's
contract-level `fixtures` metadata; subject-owned fixture bytes remain outside
the Malcolm adapter. Sorna's separate `fixture bind` command can then verify a
provider-supplied relative file against the sealed contract digest and emit a
portable `ingen.fixture-handoff/v1` artifact without adding the local path to
the contract.

 The sorna-malcolm command is only a file-format adapter. It does not run the
subject, freeze an oracle, or reinterpret Sorna results. After translation,
Sorna's existing contract validator remains the authority for the target
contract shape.

## Why

Malcolm's free-form given clauses do not yet have an executable lowering in
Sorna. Negative setup clauses now lower to an explicit list of `expect_not`
preconditions, so their meaning is retained instead of being dropped or
mistakenly applied to the target request.

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
- Request bodies contain non-empty top-level fields with recursively typed
  string, integer, boolean, object, array, or `repeat("text", count)` values.
  Repeat counts are bounded to 1 through 1,000,000, and the body field name
  `generated` is reserved for the Sorna generator marker.
- Body placeholders use {capture_name}. The adapter validates that setup
  references point to earlier captures and that target references name a
  scenario capture. Sorna resolves body placeholders as raw strings while
  retaining path escaping for URL placeholders.
- Response selectors use body.FIELD[.FIELD...] and traverse objects only.
  exists checks final-key presence, including an explicit null; array indexes
  and other selector languages are not supported.
- Stateful setup expectations are merged before lowering so status and body
  assertions are checked by one setup step. Positive requirements go under
  `expect`; each negative requirement becomes one item under `expect_not`.
- Target-rule must_not preserves Sorna's negative strength for the same
  lowerable expressions. Setup `must_not` is a precondition: if the prohibited
  expectation matches, the setup fails and the target becomes inconclusive.
  `emit "event.name"` lowers to a required event membership check.
- `isolation per scenario` accepts only a subject-owned `POST` reset path. A
  reset is run before every generated case, and a failed reset makes the case
  inconclusive instead of allowing target execution from an unknown state.
- Fixture declarations require a unique ID, owner `oracle`, non-empty purpose,
  and a lowercase SHA-256 digest. The adapter does not infer a path or copy
  fixture bytes. The Sorna provider handoff is path-based, requires a sealed
  contract, and rejects paths outside its provider root or bytes with a
  mismatched digest.
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
- [Malcolm substitutes earlier setup captures in later request bodies](malcolm-capture-interpolation.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Sorna contract specification](../../../sorna/CONTRACT-SPEC.md)
- [Notes protocol](../../../NOTES.md)
