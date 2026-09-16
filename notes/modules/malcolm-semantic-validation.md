# Malcolm validates completeness after parsing

Parsing can establish structure, but only a separate semantic pass can decide whether a structured Malcolm specification is complete enough to compile.

## Origin

The first Malcolm slice returned a typed AST while intentionally treating
expressions as opaque text. That made malformed structure visible, but it also
allowed an empty specification, a scenario without an action, or a scenario
without requirements to look successful. The next slice needed to close that
gap without prematurely designing the expression language.

## What

`validate` accepts a parsed `Specification` and returns `Ok(())` when the
model is complete. Otherwise it returns a list of `ValidationError` values.
Each error has a path such as `scenarios[0].when` and a human-readable
message. The validator currently checks names, duplicate scenario names,
non-empty setup clauses, a single required action, and at least one non-empty
requirement per scenario.

## Why

Keeping validation separate from parsing gives each phase one job. The parser
can remain focused on source shape and line-level syntax, while validation can
inspect the whole model and report relationships such as duplicate scenario
names. Returning all errors in one pass is more useful to a contract author
than stopping at the first missing field.

The validator deliberately does not interpret expressions or enforce a
particular HTTP method vocabulary. Those decisions belong to a later semantic
layer, after Malcolm's contract language has a reviewed value and operator
model.

## Example

```rust
let specification = malcolm::parse(source)?;
malcolm::validate(&specification).map_err(|errors| {
    errors
        .iter()
        .map(ToString::to_string)
        .collect::<Vec<_>>()
})?;
```

The parse step answers whether the text has the expected shape. The validation
step answers whether the resulting model is complete enough to continue.

## Gotchas

- `Ok(())` means success without a return value. The unit value `()` is Rust's
  way to say that a successful operation has nothing else to return.
- Validation errors are model paths, not source line numbers. A later compiler
  stage can preserve source spans if diagnostics need to point back to text.
- A validated model is not yet executable. Expressions remain strings, and no
  subject or implementation is contacted.
- Validation rules are part of the language contract. Adding a rule can turn a
  previously accepted specification into an invalid one and needs tests and
  review.

## Used in

- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)

## Related

- [`Malcolm's first parser preserves shape and defers meaning`](malcolm-first-parser.md)
- [`Rust ownership makes parser state explicit`](../language/rust-ownership-in-first-parser.md)
- [`Notes protocol`](../../../NOTES.md)
