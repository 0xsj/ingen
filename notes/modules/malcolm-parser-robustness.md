# Malcolm's parser accepts escaped strings and multiline nested literals

Malcolm keeps its clause grammar line-oriented while allowing request-body
objects and arrays to be written in a readable multiline form. Quoted values
also use a small, explicit escape grammar so the source can represent quotes,
backslashes, and common control characters without changing the typed AST.

## Origin

The first parser could already build recursive objects and arrays, but only
when each complete literal fit on one physical line. It also treated every
quote as a delimiter, which made values such as `say "hello"` awkward to
express. The next parser slice needed to improve authoring ergonomics without
turning free-form expressions into a new language or weakening the existing
semantic checks.

## What

Request-body values support these escapes:

~~~text
\"  quote
\\  backslash
\b  backspace
\f  form feed
\n  newline
\r  carriage return
\t  tab
~~~

The parser decodes them before constructing `Literal::String`. Unsupported
escape forms, unescaped inner quotes, literal control characters, and
incomplete escapes produce line-aware parse errors.

An object or array body value can continue across physical lines:

~~~text
given body {
  metadata = {
    "source": "import",
    "labels": [
      "docs",
      "contract"
    ]
  }
}
~~~

The continuation detector balances `{}` and `[]` only outside quoted strings.
It understands backslash escapes while scanning strings, so a brace inside a
quoted value does not close the request-body literal. Once the delimiters are
balanced, the existing literal parser validates the joined value and preserves
the original field order.

## Why

This keeps the extension small and compatible with the existing AST and IR.
The parser does not need a second literal grammar for multiline input: it only
decides when a body field is complete, then delegates syntax and type errors to
the same parser used for one-line values.

The line-oriented boundary remains useful for diagnostics. Specification,
scenario, setup, and request-body block headers still have their opening brace
on the header line, and their enclosing closing brace on its own line. A
multiline body literal reports an unclosed-delimiter error against the line
where its field assignment began.

## Gotchas

- Multiline quoted strings are not supported; only object and array literals
  may continue across lines.
- Continuation lines are joined with whitespace, so formatting whitespace is
  not part of a string value.
- Unicode escapes such as `\u0022` and other escape forms are intentionally
  rejected until their decoding and compatibility rules are defined.
- Array-index response selectors and richer provenance declarations remain
  unsupported language features. The first execution-ID provenance
  declaration is documented separately.
- Duplicate top-level and nested body field names remain errors.

## Used in

- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/README.md`](../../../malcolm/README.md)
- [`malcolm/roadmap.md`](../../../malcolm/roadmap.md)

## Related

- [`Malcolm's first parser preserves shape and defers meaning`](malcolm-first-parser.md)
- [`Malcolm's JSON IR is a transport boundary, not an evaluator`](malcolm-json-ir.md)
- [`Rust enums make a small literal grammar explicit`](../language/rust-enums-for-typed-literals.md)
- [`Notes protocol`](../../../NOTES.md)
