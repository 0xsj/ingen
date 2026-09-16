# Malcolm event assertions use an explicit HTTP signal

An event assertion is executable only when the subject exposes a reviewed
public signal for that event.

## Origin

Malcolm's example direction already used `must emit`, but the first Sorna
adapter had no event observation channel. Treating Sorna lifecycle or access
telemetry as a domain event would confuse verifier activity with subject
behavior.

## What

The supported event form is:

~~~text
must emit "document.accepted"
must_not emit "document.rejected"
must emit in order ["document.accepted", "document.queued"]
~~~

The Malcolm adapter lowers this to:

~~~json
{"events":{"required":["document.accepted"]}}
~~~

The ordered form lowers to:

~~~json
{"events":{"ordered":["document.accepted","document.queued"]}}
~~~

For the HTTP/JSON subject adapter, the runner reads the
`X-InGen-Event` response header. Multiple header values and comma-separated
names are accepted; the observation records each distinct non-empty name in
first-seen order. A `required` assertion checks membership in the current
response. An `ordered` assertion checks that the declared names occur in
relative order in the current response; unrelated events may appear between
them.

## Why

A response header is part of the public subject boundary, so the evidence can
show exactly what was observed. Sorna's `events/access.jsonl` and lifecycle
events remain separate verifier-owned telemetry; they cannot satisfy a
Malcolm domain-event assertion.

## Example

The document subject returns two `X-InGen-Event` values,
`document.accepted` and `document.queued`, with the successful
`POST /documents` response. The flow source asserts:

~~~text
when POST "/documents"
must response.status == 202
must emit "document.accepted"
must emit "document.queued"
must emit in order ["document.accepted", "document.queued"]
~~~

If the header is absent, the positive rule fails. If the same expectation has
strength `must_not`, an emitted event fails and an absent event passes.

## Gotchas

- Event assertions inspect one response; this slice does not prove delivery to
  an external broker, ordering across requests, or eventual delivery.
- Multiple events can be checked independently with `required`, or as a
  relative sequence with `ordered`. Extra events between ordered names are
  allowed. Observed names are deduplicated, so repeated occurrences of the
  same event are not a supported ordering signal.
- Event names are constrained to identifier-like segments with `.`, `:`, or
  `-` separators by the Go adapter.
- A subject that emits an event privately but does not expose the declared
  public signal has not satisfied this contract.
- Setup event assertions are positive preconditions when placed in a setup
  block; negative setup requirements remain rejected.

## Used in

- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`sorna/internal/runner/runner.go`](../../../sorna/internal/runner/runner.go)
- [`examples/document-pipeline-lab/subject/server.go`](../../../examples/document-pipeline-lab/subject/server.go)
- [`malcolm/examples/document_flow.malcolm`](../../../malcolm/examples/document_flow.malcolm)
- Makefile target `malcolm-sorna-flow-run`

## Related

- [Malcolm preserves target negative assertions as Sorna rule strength](malcolm-negative-assertions.md)
- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [A subject URL is not an isolation boundary](../concepts/a-url-is-not-an-isolation-boundary.md)
- [Notes protocol](../../../NOTES.md)
