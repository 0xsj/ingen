# The Malcolm event mutation is killed by the frozen oracle

A useful event assertion must fail when the subject removes its public event
signal, while the unchanged response behavior remains intact.

## Origin

The event assertion proof showed that a clean document creation response
contains `document.accepted`. The next risk was a vacuous green check: the
runner might record the event assertion without making it sensitive to the
subject's signal.

## What

The controlled `omit-accepted-event` defect wraps the clean document subject
and removes only `document.accepted` from the `X-InGen-Event` values for
`POST /documents`. It preserves `document.queued` and leaves the 202 status
and JSON body unchanged.

The `malcolm-sorna-flow-event-defect-run` target:

1. creates and verifies the clean seven-case baseline;
2. reuses the exact frozen flow oracle;
3. runs the event-removal subject under the same declared policies;
4. binds the run to the baseline with Sorna's mutation metadata; and
5. requires `create_document.requirement.3` to fail directly.

Sorna classifies the mutation as `killed`, so the command exits successfully
while the mutation evidence still contains the expected failed rule.

## Why

The same oracle and a passing baseline make the comparison fair. Because the
defect changes only the event header, a direct failure of the event rule is
evidence that the accepted-event assertion—not an unrelated status, body, or
queued-event assertion—detected the mutation.

## Example

~~~sh
make malcolm-sorna-flow-event-defect-run
~~~

The defect run is written under `.artifacts/malcolm-flow-event-defect-run`.
Its `run.json` records the mutation outcome and the missing event assertion.

## Gotchas

- `killed` proves sensitivity to this declared event removal; it does not
  prove that the subject emits events correctly in every workflow.
- The defect removes a transport signal, not an external broker publication.
  Async delivery, ordering, and consumer receipt remain outside this slice.
- The multi-event header remains observable after the mutation: the queued
  event still passes while the accepted event fails.
- The mutation target must be a rule ID from the frozen oracle and the
  baseline must pass with matching contract, oracle, and policy identities.
- The subject policy still permits only the flow's declared local HTTP port;
  the defect reuses that port after the clean baseline has shut down.

## Used in

- [`examples/document-pipeline-lab/defects/omit-accepted-event/`](../../../examples/document-pipeline-lab/defects/omit-accepted-event/)
- [`sorna/internal/runner/runner.go`](../../../sorna/internal/runner/runner.go)
- Makefile target `malcolm-sorna-flow-event-defect-run`
- [`malcolm/examples/document_flow.malcolm`](../../../malcolm/examples/document_flow.malcolm)

## Related

- [Malcolm event assertions use an explicit HTTP signal](malcolm-event-assertions.md)
- [Mutation results separate target sensitivity from setup fallout](sorna-mutation-result-model.md)
- [A killed mutation proves sensitivity, not correctness](../concepts/a-killed-mutation-proves-sensitivity.md)
- [Notes protocol](../../../NOTES.md)
