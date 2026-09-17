# Malcolm carries typed request bodies and stateful setup across the boundary

Executable request data needs a typed source shape before a cross-language
adapter can safely hand it to Sorna.

## Origin

Malcolm's original `given` clause stored free-form text. That preserved source
shape, but it could not become an HTTP request without guessing. Sorna already
supports JSON request bodies, setup requests, response assertions, and
captures, so the next slice makes those pieces explicit in Malcolm.

## What

Malcolm now supports a small executable subset:

~~~text
scenario read_document {
  state document_accepted
  setup accept_document {
    given body {
      name = "welcome.md"
    }
    when POST "/documents"
    must response.status == 202
    must response.body.id exists
    capture document_id = response.body.id
  }
  when GET "/documents/{document_id}"
  must response.status == 200
}
~~~

Nested values use inline JSON-like literals:

~~~text
given body {
  metadata = {"source": "malcolm", "reviewed": true}
  tags = ["docs", "contract"]
}
~~~

Deterministic repeated strings can be used for bounded request-size cases:

~~~text
scenario reject_oversized_document {
  given body {
    name = "too-large.txt"
    content = repeat("a", 4097)
  }
  when POST "/documents"
  must response.status == 413
}
~~~

The Rust AST represents object fields as ordered names plus a recursive
`Literal` enum. Inline objects, arrays, and repeat generators retain their
typed meaning through the IR. The Go adapter lowers the
scenario request to `given.body`, the setup sequence to `given.setup`, the
state label to `given.state`, and capture selectors to Sorna's
`body.FIELD` form. Setup requirements are merged into one executable setup
expectation; negative setup requirements remain independent `expect_not` items.

## Why

The source contract must say which values are executable. A typed body block
can be serialized and executed without interpreting prose. A setup block also
keeps state creation, setup verification, capture, and target execution in
the Sorna runner, where those lifecycle and evidence semantics already live.

The adapter still rejects free-form `given` text and unsupported assertions.
This keeps a Malcolm source contract from silently becoming a weaker Sorna
contract.

## Example

~~~sh
make malcolm-sorna-flow-contract
~~~

The target compiles `malcolm/examples/document_flow.malcolm`, translates it,
and asks Sorna's validator to accept the resulting `ingen.contract/v1`
document.

## Gotchas

- The current body grammar supports non-empty top-level fields whose values
  are strings, signed 64-bit integers, booleans, inline objects, arrays, or
  `repeat("text", count)` generators.
- Nested objects and arrays may contain the same scalar values recursively.
  Repeat generators may appear at any body value position. Their count must
  be between 1 and 1,000,000, and the generator value must be a string.
- The body field name `generated` is reserved because Sorna uses that marker
  in the cross-language contract representation.
- String body values may contain a capture placeholder such as
  {document_name}. The Rust validator checks that the capture is available
  from an earlier setup, and Sorna substitutes the captured text at request
  time. Body substitution is not URL escaping or expression evaluation.
- A `state` label requires at least one setup; otherwise it would be metadata
  with no executable way to establish the state.
- Each setup needs a request, at least one lowerable requirement, and unique
  capture names. Target-rule `must_not` and negative setup requirements are
  supported for the same lowerable expressions.
- Sorna runs setup once per generated rule case. Repeating a setup for separate
  requirements is deliberate because each rule remains an independent oracle
  case.
- The generated contract is validated by the flow target; behavioral execution
  belongs to the separate flow-run target and its dedicated policies.
- Sorna materializes repeat generators while freezing the oracle, so the
  frozen oracle contains the concrete request value used for replay.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`malcolm/examples/document_flow.malcolm`](../../../malcolm/examples/document_flow.malcolm)
- [`malcolm/examples/document_boundary.malcolm`](../../../malcolm/examples/document_boundary.malcolm)
- `Makefile` target `malcolm-sorna-flow-contract`
- `Makefile` target `malcolm-sorna-boundary-run`

## Related

- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [Malcolm substitutes earlier setup captures in later request bodies](malcolm-capture-interpolation.md)
- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [Rust enums make a small literal grammar explicit](../language/rust-enums-for-typed-literals.md)
- [Notes protocol](../../../NOTES.md)
