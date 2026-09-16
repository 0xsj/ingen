# The contract schema must agree with the runtime identity

Sorna's contract loader previously checked only that `contract.schema` was a
non-empty string. That allowed a legacy `sorna.contract/v1` value to pass even
though the current alpha boundary is `ingen.contract/v1`. The mismatch was
visible in the old Todo example and in the illustrative contract specification.

The validator now owns the identity constant and rejects every other schema
value. The example and specification use the same identity, and the published
[`ingen.contract-v1.schema.json`](../../sorna/spec/ingen.contract-v1.schema.json)
gives non-Go tooling the corresponding outer shape.

The JSON Schema is deliberately structural. It closes the top-level envelope
and contract fields, while nested expectation predicates remain extensible so
the schema does not pretend to enumerate every future adapter expression.
Runtime validation still owns cross-field and execution semantics: unique rule
IDs, subjects for executable rules, stateful setup, generator bounds, fixture
hashes, and canonical sealing behavior.

The useful invariant is therefore:

```text
published schema identity == runtime schema identity == example/spec identity
```

The focused contract, schema, and CLI validation tests exercise that invariant.
