# A provenance value is a history node, not a mutable request bag

Every provenance transition returns a new value so the prior execution remains
available as evidence of what happened before the transition.

## Origin

Implementing `Start`, `Child`, `Retry`, and `Replay` exposed the central
identity distinction in Amber: logical work can continue across executions,
while each execution must remain individually inspectable.

## What

`work_id` names the logical unit of work and `execution_id` names one concrete
run. A child starts new logical work while inheriting the flow's
`correlation_id`; a retry keeps the same work and increments `attempt`; a replay
keeps the same work but does not increment the retry attempt. Every transition
also gets a fresh execution ID and records its immediate cause.

The Go value keeps its fields private, and the TypeScript class exposes copies
through getters. This makes mutation an explicit transition rather than an
ordinary field assignment.

## Why

A mutable request-shaped struct would let middleware overwrite identity or
attribution after a record had already been observed. That erases the
difference between the prior execution and the new one, makes retry history
ambiguous, and allows callers to bypass validation. Returning a new value makes
the causal chain visible at the call site and preserves the parent for audit or
debugging.

## Example

```text
root:   work=A, execution=E1, attempt=1
retry:  work=A, execution=E2, caused-by=E1, attempt=2
replay: work=A, execution=E3, caused-by=E1, attempt=1
child:  work=B, execution=E4, caused-by=E1, attempt=1
```

## Gotchas

- A replay is intentional re-execution, not a retry; its `attempt` stays the
  same and it receives a separate `replay_id`.
- Correlation groups related flow work; it does not replace either work or
  execution identity.
- Production transitions use secure random UUIDv4 generation, while an
  injectable ID factory exists so conformance tests can assert exact transition
  outputs without making production identity deterministic.
- Immutability only holds if accessors return copies of slices and nested
  objects, which is why both implementations clone references and attribution.

## Used in

- [`go/provenance.go`](../../go/provenance.go)
- [`go/provenance_test.go`](../../go/provenance_test.go)
- [`typescript/src/provenance.ts`](../../typescript/src/provenance.ts)
- [`typescript/src/provenance.test.ts`](../../typescript/src/provenance.test.ts)
- [`spec/v1.md`](../../spec/v1.md)

## Related

- [The shared contract must precede both SDKs](001-spec-first-foundation.md)
- [Context propagation should be scoped by construction](003-scoped-context-and-restoration.md)
