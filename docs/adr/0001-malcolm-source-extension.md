# ADR-0001: Use `.malc` for Malcolm source files

Status: Accepted  
Date: 2026-09-17

## Context

Malcolm source files were using the long `.malcolm` extension. The extension
is readable, but it makes ordinary contract filenames unnecessarily verbose,
especially in the fresh-project layout where the language source sits beside
the generated IR and Sorna contract.

Using `.ml` would be shorter but is already strongly associated with ML-family
languages such as OCaml and Standard ML. That ambiguity would make editor,
tooling, and repository discovery less clear.

## Decision

Use `.malc` as the canonical source extension for the Malcolm specification
language.

Examples:

```text
contract.malc
document-api.malc
.ingen/contract/spec.malc
```

Generated IR, sealed contracts, oracles, and evidence retain their existing
JSON and directory names.

## Consequences

- Malcolm source files are concise while retaining a recognizable language
  identity.
- The CLI, examples, Make targets, notes, and fresh-project scaffold use one
  consistent extension.
- Existing `.malcolm` files need to be renamed; this is a source-layout change,
  not a grammar or IR compatibility change.
- Editor support can later associate `.malc` with Malcolm without colliding
  with common ML language tooling.
