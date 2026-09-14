# A producer report can cross the CI boundary without losing its semantics

Sorna now emits the shared `ingen.ci-result/v1` envelope with:

- `tool: "sorna"` and `kind: "behavioral-verification"`;
- the gate's `passed`/`failed` status and matching `0`/`1` exit code;
- a `report` containing the unchanged `ingen.gate/v1` result;
- an `explanation` containing `sorna.gate-explanation/v1`; and
- `inputs` that index the verified manifest, run or oracle, event streams, and
  policy copy when those files exist.

The shared Go package validates only envelope-owned facts: schema, status,
exit-code mapping, timestamps, source identity, file-reference shape, and the
presence of producer JSON. It treats reports and explanations as opaque JSON.
That is the important boundary: Nublar can combine Sorna and Paddock results
without importing either verifier or reinterpreting a mutation result.

## Why the report stays nested

Sorna's contract verdict and mutation outcome have different meanings. A killed
mutation has an expected nested contract failure, but the mutation campaign
itself passes because the contract detected the changed behavior. The envelope
therefore exposes only the final Sorna gate status as authoritative and keeps
the detailed reasoning in the producer-owned report.

## Command

```sh
sorna gate --format ci-result .artifacts/document-pipeline-run \
  > .artifacts/document-pipeline-ci-result.json
```

`make sorna-ci-result` is the repository shortcut. The result file is written
before the command returns the gate exit code, so a failed behavioral gate can
still be collected by CI.

When verification itself fails, the same format emits `status: "error"` and
`exit_code: 2` with the diagnostic in `error`; this keeps an invalid evidence
bundle distinguishable from a valid bundle whose behavioral gate failed.

## Limits

The envelope is an interoperability boundary, not a new evidence claim. The
input hashes establish references to bundle files; they do not prove that the
subject was isolated, that observation was continuous, or that the contract is
complete. The shared package is intentionally additive while Paddock's
existing producer type remains in place; a later migration can make both
producers consume the same Go representation.
