# Malcolm's first parser preserves shape and defers meaning

A typed AST gives Malcolm a stable contract shape while leaving expressions as independent-language text until their semantics are deliberately specified.

## Origin

Malcolm had a design README but no implementation. The first slice needed to
prove that the readable `spec`/`scenario` syntax could become structured data
without jumping straight to an evaluator or coupling the language to Sorna.

## What

The parser accepts a deliberately small line-oriented syntax: a specification
has an optional version and subject, and scenarios contain `given`, `when`,
`must`, and `must_not` clauses. The result is a set of Rust structs and enums.
Expressions are stored as strings for now. Request-body object and array
literals may continue across lines, while the surrounding clauses remain
line-oriented. The parser checks structure and reports a `ParseError` with a
source line, but it does not decide what an expression such as
`response.status == 202` means.

## Why

Parsing and evaluating at the same time would make the first syntax experiment
answer too many questions at once. Keeping the expression text intact lets a
future semantic layer choose the right value model, operators, and adapters
after the contract shape has been reviewed. It also keeps the first
intermediate representation independent of the implementation language being
verified.

A line-oriented parser is an intentional prototype boundary, not a claim that
newlines will remain significant forever. The initial examples put one clause
per line, so this form makes errors easy to locate and keeps the parser free of
dependencies while the language is still unsettled. The one controlled
exception is nested request data: an object or array can be accumulated across
lines before the existing typed literal parser validates it.

## Example

```text
spec document_api v1 {
  subject "documents"
  scenario submit_document {
    given document is valid
    when POST "/documents"
    must response.status == 202
  }
}
```

This becomes a `Specification` containing one `Scenario`; the `when` clause
becomes a typed method/path pair, while the `given` and `must` text remains
available for the next compilation stage.

## Gotchas

- Successful parsing does not mean the specification is semantically valid.
  There is not yet a check for duplicate scenario names, missing `when`
  clauses, or a meaningful expression grammar.
- Specification, scenario, setup, and request-body block headers still need
  their opening brace on the header line, and their enclosing closing brace on
  its own line. Nested object and array delimiters may be spread across lines.
- Quoted strings support the basic escapes `\\`, `\"`, `\\n`, `\\r`, `\\t`, `\\b`,
  and `\\f`. Literal newlines inside quoted strings and other escape forms such
  as Unicode escapes are not supported.
- A multiline body value must begin after one `FIELD =` assignment. The parser
  joins continuation lines with whitespace and reports an unclosed object or
  array against the value's starting line.
- Raw expressions are not safe to execute. They are data until a later,
  explicitly designed semantic layer interprets them.
- The parser must remain independent of implementation worktrees; adding
  source-aware shortcuts would undermine Malcolm's contract boundary.

## Used in

- [`malcolm/src/ast` and `malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/README.md`](../../../malcolm/README.md)

## Related

- [`Rust ownership makes parser state explicit`](../language/rust-ownership-in-first-parser.md)
- [`The contract can cross language boundaries`](../concepts/the-contract-can-cross-language-boundaries.md)
- [`Sorna's first vertical slice`](../../../sorna/FIRST-VERTICAL-SLICE.md)
- [`Notes protocol`](../../../NOTES.md)
