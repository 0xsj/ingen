# InGen examples

The examples are controlled subjects for learning and verifying InGen itself.
They are not production templates and are not tied to one implementation
language unless an experiment explicitly says so.

Each example should make the same chain visible:

```text
contract → oracle → subject → deliberate defects → evidence → notes
```

The contract describes public behavior. The subject may be written in Go,
Python, TypeScript, or another language supported by an adapter. Changing the
subject language should not change the contract's meaning.
