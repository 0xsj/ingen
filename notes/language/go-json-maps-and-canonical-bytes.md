# Go's JSON map ordering is useful, but canonical bytes are still a contract

Go's JSON encoder gives maps a stable key order, but reproducible artifact hashes still require an explicit serialization policy and a test that protects it.

## Origin

Implementing the first Sorna contract sealer in Go and deciding how a YAML
contract becomes bytes that can be hashed and replayed.

## What

The contract is represented as nested Go maps because rules and predicates are
extensible. `encoding/json` sorts string map keys while encoding, so equivalent
documents with different map insertion order can produce the same canonical
JSON. Sorna also fixes newline, HTML-escaping, and supported scalar behavior so
the hash is over a deliberate artifact rather than an incidental printout.

## Why

Using a map is useful for an evolving contract schema, but map iteration order
must not leak into a sealed artifact. A hand-written serializer would add more
code and another place for a mismatch. The standard encoder is a good substrate
provided its behavior is made visible by tests and the serialization policy is
not left implicit.

## Example

Two documents constructed with the same fields in different insertion orders
must have identical canonical bytes and therefore identical SHA-256 digests.

## Gotchas

- JSON numbers, YAML numbers, strings, and timestamps must not be allowed to
  change type silently during parsing.
- A hash proves that the bytes stayed the same; it does not prove the contract
  was correct.
- Formatting or changing an unsealed source document creates a different sealed
  artifact even when a human considers the change cosmetic.

## Used in

- `sorna/internal/contract` canonicalization and sealing
- future InGen evidence manifests

## Related

- [`sorna/CONTRACT-SPEC.md`](../../../sorna/CONTRACT-SPEC.md)
- [`sorna/EVIDENCE-SPEC.md`](../../../sorna/EVIDENCE-SPEC.md)
- [`NOTES.md`](../../../NOTES.md)
