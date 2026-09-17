# Malcolm makes scenario reset semantics explicit

Independent oracle cases must not silently share state. A stateful setup can
prepare one case, but it does not prove that a later case starts from the same
initial condition.

## Origin

Malcolm's Sorna flow executes several generated rules against one managed
subject process. The document fixture stores documents in memory, so a case
that creates state could affect a later case unless the fixture is reset.

## What

Malcolm supports one narrow isolation declaration:

~~~text
isolation per scenario {
  reset POST "/__malcolm/reset"
}
~~~

The Rust AST and `malcolm.ir/v1` preserve the scope and reset request. The
Sorna adapter copies it into every executable rule's `given.isolation` object.
Before each case, the HTTP runner invokes the declared reset operation, records
its request and response, and only then executes setup and target requests.

A successful 2xx reset permits the case to continue. A reset response outside
2xx is recorded as `inconclusive`, and the target request is not sent. A reset
transport error is also inconclusive because the runner cannot establish the
declared starting boundary.

## Ownership

The reset operation is owned by the subject fixture. It is a test/control hook,
not an implicit Malcolm transition and not part of the business API. Malcolm
declares when the hook is required; Sorna invokes and records it; the subject
decides what resources the hook actually clears.

## Why

This keeps state creation and cleanup visible at the public boundary. It also
makes the important claim reviewable: the runner observed a successful reset
before a case, rather than inferring isolation from a state label or from the
fact that a process was restarted.

## Gotchas

- The current scope is `per scenario`, which means one reset before every
  generated rule case. Run-level and setup-level scopes are unsupported.
- Only `POST` reset requests with an absolute path are accepted.
- The runner records the reset response, but cannot prove that private
  databases, queues, caches, or files were cleared.
- Subject-owned fixture hooks must not be confused with Malcolm-declared input
  fixture files. File fixture declarations remain a separate future slice.
- A reset failure is `inconclusive`, not a target assertion failure, because
  the target was not evaluated from a known starting state.

## Used in

- [`malcolm/src/lib.rs`](../../../malcolm/src/lib.rs)
- [`malcolm/src/parser.rs`](../../../malcolm/src/parser.rs)
- [`malcolm/src/semantic.rs`](../../../malcolm/src/semantic.rs)
- [`malcolm/src/ir.rs`](../../../malcolm/src/ir.rs)
- [`sorna/internal/malcolm/adapter.go`](../../../sorna/internal/malcolm/adapter.go)
- [`sorna/internal/runner/runner.go`](../../../sorna/internal/runner/runner.go)
- [`document-pipeline fixture`](../../../examples/document-pipeline-lab/subject/server.go)
- [`malcolm/examples/document_flow.malcolm`](../../../malcolm/examples/document_flow.malcolm)

## Related

- [Malcolm carries typed request bodies and stateful setup](malcolm-request-body-lowering.md)
- [A state label is not an executable transition](../concepts/a-state-label-is-not-a-transition.md)
- [Managed subject lifecycle improves reproducibility without proving isolation](sorna-managed-subject-lifecycle.md)
- [A managed subject needs its own isolation policy](sorna-subject-isolation.md)
- [Notes protocol](../../../NOTES.md)
