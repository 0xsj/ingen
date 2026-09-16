# The Malcolm stateful flow produces evidence through separate Sorna policies

A stateful contract becomes behavioral evidence only when Sorna freezes and
executes it under independently declared oracle and subject boundaries.

## Origin

The request-body lowering proof established a valid Sorna contract, but
contract validation alone did not send a body, execute setup, resolve a
capture, or verify the resulting target request. The existing document subject
already implements the POST and GET flow needed for that proof.

## What

The `malcolm-sorna-flow-run` target:

1. compiles `malcolm/examples/document_flow.malcolm`;
2. translates and validates `malcolm.ir/v1` as an `ingen.contract/v1` draft;
3. seals the generated contract;
4. freezes its oracle under the dedicated flow oracle policy;
5. builds the clean document subject;
6. runs the oracle under the separate flow subject policy; and
7. verifies the resulting evidence bundle.

The flow has seven executable rule cases. The create cases send a JSON body
directly. The read cases first POST a document, assert the setup response,
capture `body.id`, substitute it into
`/documents/{document_id}`, and assert the queued result. The final read case
also proves that a prohibited `response.body.error exists` expectation is
treated as a negative rule. One create case also proves that the subject's
`X-InGen-Event` response signals satisfy `must emit "document.accepted"` and
`must emit "document.queued"`.

## Why

This closes the gap between “the adapter emitted a structurally valid
contract” and “Sorna observed the contract behavior.” The oracle policy cannot
invoke the subject and the subject policy cannot read Malcolm or the generated
contract/oracle, so the run records a meaningful public-boundary separation.

## Example

~~~sh
make malcolm-sorna-flow-run
~~~

The run output is written under `.artifacts/malcolm-flow-run`. Its oracle,
sealed contract, and subject artifacts use the corresponding
`.artifacts/malcolm-flow-*` paths.

## Gotchas

- A passing flow run proves these seven declared cases against this subject
  boundary; it does not prove the entire document API.
- Setup runs once per generated rule case. Each rule therefore creates its own
  document and captures its own ID.
- The subject policy permits only inbound localhost traffic on port 8080 and
  denies Malcolm and generated contract/oracle paths.
- macOS host enforcement may require the local Seatbelt permission needed by
  Sorna's sandbox backend. An unavailable backend must not be reported as a
  host-enforced pass.
- The flow run uses the frozen oracle, not the draft contract or Malcolm source
  at execution time.

## Used in

- [`malcolm/examples/document_flow/oracle-policy.yaml`](../../../malcolm/examples/document_flow/oracle-policy.yaml)
- [`malcolm/examples/document_flow/subject-policy.yaml`](../../../malcolm/examples/document_flow/subject-policy.yaml)
- `Makefile` target `malcolm-sorna-flow-run`
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`sorna/internal/runner/runner.go`](../../../sorna/internal/runner/runner.go)

## Related

- [Malcolm carries typed request bodies and stateful setup across the boundary](malcolm-request-body-lowering.md)
- [The Malcolm contract must be sealed before Sorna can produce behavioral evidence](malcolm-sorna-run.md)
- [A subject URL is not an isolation boundary](../concepts/a-url-is-not-an-isolation-boundary.md)
- [Notes protocol](../../../NOTES.md)
