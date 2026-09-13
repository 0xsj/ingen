# A generated boundary must be finite and reproducible

An oracle may generate a boundary input, but the contract must state how many values it creates and how the same values are regenerated.

## Origin

The first document-pipeline contract needed a document larger than the 4096-byte
limit without embedding thousands of characters in the YAML file.

## What

The contract uses an explicit `generated` value with a `repeat` kind, a value,
and a count. This keeps the boundary case readable while making its size a
contract fact that an oracle runner can reproduce.

## Why

An unbounded generator makes a run impossible to compare or replay. An implicit
boundary value also hides whether the oracle actually exercised the edge. The
small generator description keeps the intent in the contract and leaves the
implementation language out of it.

## Example

```yaml
content:
  generated:
    kind: repeat
    value: a
    count: 4097
```

## Gotchas

- The generator syntax is part of the contract schema and must be versioned.
- A count can be finite but still exceed resource limits; the runner needs a
  configured safety ceiling.
- The generated value must be recorded in the case manifest or be derivable
  from a recorded seed and generator configuration.

## Used in

- `examples/document-pipeline-lab/contract/contract.yaml`
- future Sorna boundary and property-case generation

## Related

- [`sorna/ORACLE-SPEC.md`](../../../sorna/ORACLE-SPEC.md)
- [`sorna/CONTRACT-SPEC.md`](../../../sorna/CONTRACT-SPEC.md)
- [`NOTES.md`](../../../NOTES.md)
