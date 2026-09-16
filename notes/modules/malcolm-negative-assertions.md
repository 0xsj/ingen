# Malcolm preserves target negative assertions as Sorna rule strength

A negative assertion is a prohibition on an observable pattern, not a request
to invert arbitrary source text.

## Origin

Malcolm already parsed must_not, but the adapter rejected it because Sorna's
runner evaluated every rule as a positive expectation. Sorna's contract and
oracle schemas already carry strength: must_not, so the missing behavior
belonged in the execution boundary.

## What

For the supported target expressions, Malcolm lowers:

~~~text
must_not response.body.error exists
~~~

to a Sorna rule with the same executable expectation and strength: must_not.
The runner evaluates the expectation normally, then treats a complete match as
a contract failure. If any part of the prohibited expectation does not match,
the negative rule passes.

## Why

Keeping the expectation shape unchanged makes the evidence explainable: the
record still shows which observable pattern was checked. Applying the
prohibition at the runner preserves Sorna's existing observation and
assertion machinery instead of inventing a second response evaluator in the
Malcolm adapter.

## Example

~~~text
scenario health_has_no_error {
  when GET "/healthz"
  must response.status == 200
  must_not response.body.error exists
}
~~~

In the document flow, an absent error field passes this rule. A response
containing error makes the prohibited expectation match and fails the rule.

## Gotchas

- This slice supports must_not only for target rules and the same status,
  top-level response presence, and top-level response equality expressions as
  must.
- Setup `must_not` requirements are supported as preconditions. They lower to
  separate Sorna `expect_not` list items; if any prohibited expectation
  matches, setup fails and the target rule is marked inconclusive. Captures
  happen only after all positive and negative setup checks pass.
- A negative rule with no executable assertion is an execution error rather than
  a vacuous pass.
- A passing negative assertion proves only that the prohibited pattern was not
  observed for that request; it does not prove the opposite positive behavior.
- Event absence and event emission now use a separate, explicit HTTP subject
  signal; Sorna lifecycle and access telemetry do not satisfy domain-event
  assertions.

## Used in

- [sorna/internal/runner/runner.go](../../../sorna/internal/runner/runner.go)
- [sorna/internal/runner/runner_test.go](../../../sorna/internal/runner/runner_test.go)
- [sorna/internal/malcolm/adapter.go](../../../sorna/internal/malcolm/adapter.go)
- [malcolm/examples/document_flow.malcolm](../../../malcolm/examples/document_flow.malcolm)
- Makefile target malcolm-sorna-flow-run

## Related

- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [Malcolm carries typed request bodies and stateful setup across the boundary](malcolm-request-body-lowering.md)
- [The contract can cross language boundaries](../concepts/the-contract-can-cross-language-boundaries.md)
- [Notes protocol](../../../NOTES.md)
