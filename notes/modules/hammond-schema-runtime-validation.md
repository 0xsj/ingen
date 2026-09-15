# A governance schema must be enforced at the load boundary

Hammond's machine-readable schema and its Go domain validator must reject
unknown fields before a record becomes a trusted governance object.

## Origin

The v1 JSON Schema uses closed objects, but Go's ordinary `json.Unmarshal`
silently ignores fields that are not represented in the destination struct.

## What

Hammond decodes records and events with unknown-field rejection before running
domain validation. This keeps the wire artifact and the in-memory governance
model aligned: a producer cannot add an unrecognized field that a reader
silently discards while still treating the result as the same record.

## Why

Governance records are audit inputs, not loose application payloads. Silently
discarding a field can hide a producer/consumer version mismatch or make a
review decision appear to have been preserved when the reader did not
understand part of the artifact. The schema's `additionalProperties: false`
only helps when the load boundary actually enforces the same rule.

## Gotchas

- Strict decoding does not replace semantic validation; both are required.
- Event fields still need context checks, such as matching the contract digest.
- A valid JSON document may still be invalid Hammond state.
- Adding a wire field is an interface change and needs an intentional schema
  version or compatibility decision.

## Used in

- `hammond/internal/governance`
- `hammond/internal/store`
- `hammond/cmd/hammond`
- `hammond/spec/ingen.hammond-governance-v1.schema.json`

## Related

- [Hammond governance specification](../../../hammond/GOVERNANCE-SPEC.md)
- [Hammond approval governs identified bytes, not behavior](hammond-governance-artifact-identity.md)
