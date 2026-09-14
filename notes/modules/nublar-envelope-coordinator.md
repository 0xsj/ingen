# A coordinator should compose verdicts, not reinterpret findings

Nublar's first implementation is deliberately small. It loads one or more
`ingen.ci-result/v1` artifacts, validates the shared envelope, preserves each
complete producer result, and computes a coordinator status using only the
envelope's status field.

The composition rule is:

```text
error > failed > passed
```

The corresponding aggregate exit codes are `2`, `1`, and `0`. This means a
Sorna mutation report and a Paddock architecture report can travel through the
same CI workflow without Nublar knowing what “killed” or “violated” means.

## Why preserve the complete input

An aggregate summary without the original reports would make the coordinator a
lossy boundary and encourage it to recreate producer semantics. Nublar instead
writes `ingen.nublar-result/v1` with each complete input under `results`, along
with the input path. Consumers can inspect or retain producer detail while
using the aggregate status for the workflow decision.

## Command

```sh
go run ./nublar/cmd/nublar aggregate \
  --output .artifacts/nublar-result.json \
  .artifacts/document-pipeline-ci-result.json \
  .artifacts/paddock-ci-result.json
```

Duplicate input paths are rejected. An invalid input stops aggregation rather
than producing a partial result, because a partial coordinator decision would
be ambiguous.

## Limits

This is not yet a pull-request service, artifact store, scheduler, or policy
engine. It is the first executable proof that Sorna and future producers can
share a CI boundary without forcing an artificial merger of their internals.
