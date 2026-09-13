# Mutation results separate target sensitivity from setup fallout

Sorna can call a mutation `killed` when at least one declared target rule fails directly; dependent setup failures remain visible but do not become kills.

## Origin

The first `status-200-create` defect changed successful document creation from
202 to 200. The create rule failed directly, while stateful rules that used
creation as setup became inconclusive.

## What

Each mutation declares an ID, plane, description, and expected rule IDs. The
classifier compares those targets with rule statuses and reports `killed` when
at least one target fails directly, otherwise `survived` or `inconclusive` as
appropriate. It also lists directly failed rules, cascading
inconclusive rules, unaffected passing rules, and expected rules that were not
observed.

## Why

An aggregate non-zero run is too coarse for mutation evidence. Counting every
setup cascade as a kill would exaggerate oracle sensitivity, while hiding the
cascade would make the run difficult to explain. Declaring expected targets
before execution makes the causal claim reviewable.

## Example

```json
{
  "id": "status-200-create",
  "expected_rule_ids": ["document.create.valid.accepted"]
}
```

The clean subject passes that target; the defect subject fails it, so the
mutation outcome is `killed`.

## Gotchas

- `killed` proves that the declared behavior change was observable, not that
  the whole subject is correct.
- A target that is only `inconclusive` must not be counted as killed.
- Expected rule IDs are part of the mutation claim and must be recorded before
  the run begins.

## Used in

- [`sorna/internal/mutation`](../../sorna/internal/mutation/)
- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`status-200-create defect`](../../examples/document-pipeline-lab/defects/status-200-create/)

## Related

- [`A killed mutation proves sensitivity, not correctness`](../concepts/a-killed-mutation-proves-sensitivity.md)
- [`Sorna mutation specification`](../../sorna/MUTATION-SPEC.md)
- [`The first Sorna runner accepts a subject URL instead of a subject package`](sorna-http-runner.md)
