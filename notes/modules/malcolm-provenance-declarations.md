# Malcolm's first provenance declaration checks Amber execution identity

Malcolm can declare one small provenance obligation at the public HTTP
boundary. The declaration remains typed in Malcolm, is lowered into Sorna's
expectation shape, and is evaluated from the subject response. Amber remains
the owner of full provenance semantics.

## Origin

The original Malcolm direction named application-level relationships such as
request, execution, and tenant identity, but the parser had no provenance
model. Amber already defines the immutable value and its transitions, while
Sorna already records public HTTP observations and evidence. The smallest
useful slice is therefore a response-level check for an Amber
`Amber-Provenance` value exposing `execution_id`.

## What

The supported syntax is:

~~~text
provenance {
  must create execution_id
}
~~~

The block is specification-wide and applies to every target scenario response.
It does not apply to setup responses in this first slice. The Rust AST stores
the requirement as typed `Create` and `ExecutionId` values. The
`malcolm.ir/v1` representation is:

~~~json
{
  "provenance": {
    "requirements": [
      {"kind": "create", "field": "execution_id"}
    ]
  }
}
~~~

The Sorna adapter adds this expectation to each target rule:

~~~json
{"provenance":{"required":["execution_id"]}}
~~~

The HTTP runner observes the public `Amber-Provenance` response header,
decodes its unpadded base64url JSON payload, and records the execution ID and
payload digest in the observation. A missing, malformed, or empty execution ID
produces a failed `provenance.execution_id` assertion.

## Ownership

| Concern | Owner |
| --- | --- |
| Declare the required relationship | Malcolm |
| Create, validate, and transition the Amber value | Amber and the subject |
| Observe the HTTP response and classify the assertion | Sorna |
| Retain evidence bytes and hashes | Lockwood/Sorna evidence boundary |
| Collect the producer result | Nublar |

## Why

This creates a real contract/evidence mapping without duplicating Amber's
domain model in Malcolm or Sorna. The run evidence shows whether the public
subject response carried the declared identity, while the Amber library still
owns UUID validity, causation, attribution, retries, replay, context
propagation, and durable storage.

## Gotchas

- The declaration checks target responses only; setup propagation is deferred.
- Sorna checks the transport shape and required field presence. It does not
  claim to validate the complete Amber v1 object.
- Malcolm does not create an execution ID or inject an incoming header.
- A response header is a public signal, not proof of end-to-end causal
  correctness or trusted provenance.
- Richer requirements for work, correlation, tenant, causation, and event links
  remain unsupported until their evidence model is agreed.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`sorna/internal/runner/runner.go`](../../../sorna/internal/runner/runner.go)
- [`malcolm/examples/document_provenance.malcolm`](../../../malcolm/examples/document_provenance.malcolm)

## Related

- [`Amber core specification`](../../../amber/spec/v1.md)
- [`Amber HTTP adapter`](../../../amber/go/adapters/http/README.md)
- [`Malcolm event assertions`](malcolm-event-assertions.md)
- [`Malcolm's JSON IR`](malcolm-json-ir.md)
- [`Notes protocol`](../../../NOTES.md)
