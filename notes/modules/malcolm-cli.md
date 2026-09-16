# Malcolm's CLI keeps the compile pipeline visible

The first Malcolm command should expose parsing, validation, and IR emission as one inspectable pipeline with errors that tell the contract author which phase failed.

## Origin

Malcolm's parser, semantic validator, and JSON IR were initially callable only
as Rust library functions. That was enough for unit tests but not enough for a
person or another InGen tool to try the language without writing Rust code.

## What

The malcolm binary accepts one input path. It reads a file, or reads from
standard input when the path is -, then parses the source, validates the
model, and writes malcolm.ir/v1 JSON to standard output. -o and --output write
the result to a named file instead. Usage errors, file I/O errors, parse
errors, and validation errors remain distinct in the command's message.

## Why

The CLI is intentionally a thin adapter around the library. If it reimplemented
parsing or validation, library users and command-line users could observe
different language rules. Keeping the pipeline in the library also gives the
future Sorna integration a direct API while the CLI remains useful for humans,
shell scripts, and fixtures.

The command writes JSON only after parsing and validation succeed. A malformed
or incomplete contract therefore cannot be mistaken for a usable IR artifact.

## Example

~~~sh
cargo run --manifest-path malcolm/Cargo.toml -- \
  malcolm/examples/document_api.malcolm \
  --output .artifacts/document-api.ir.json
~~~

The checked-in example is deliberately limited to the currently supported
syntax. The README's future-facing provenance and mutation sections are not
accepted by this first CLI slice yet.

## Gotchas

- The CLI emits JSON to stdout by default; diagnostics go to stderr and the
  process exits nonzero on failure.
- - means stdin only when used as the input path. It is not an output path.
- The output file is replaced by the normal filesystem write operation.
  Atomic artifact publication is a later hardening step if CLI output becomes
  a custody or CI boundary.
- A successful command proves only that the source was parsed, validated, and
  serialized. It does not execute a subject or verify behavior.

## Used in

- [malcolm/src/main.rs](../../../malcolm/src/main.rs)
- [malcolm/examples/document_api.malcolm](../../../malcolm/examples/document_api.malcolm)
- [malcolm/README.md](../../../malcolm/README.md)

## Related

- [Malcolm's JSON IR is a transport boundary, not an evaluator](malcolm-json-ir.md)
- [Malcolm validates completeness after parsing](malcolm-semantic-validation.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Notes protocol](../../../NOTES.md)
