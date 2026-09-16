# Nublar CI gate example

`nublar-ci-gate.sh` is a provider-neutral consumer example. It assumes that a
separate producer workflow has already written complete `ingen.ci-result/v1`
envelopes at the paths declared by the Nublar workflow.

From the repository root:

```sh
bash nublar/examples/consumer/nublar-ci-gate.sh \
  --workflow nublar/workflows/document-pipeline.yaml \
  --root .artifacts \
  --store .artifacts/nublar-runs \
  --output .artifacts/nublar-run.json \
  --external-system github-actions \
  --external-id build-42 \
  --attempt 3
```

The example writes the complete run and a compact decision projection. It
prints the generated or supplied local `run_id` and returns Nublar's decision
code: `0` for passed, `1` for failed, and `2` for a collection or export
error. The script derives that code from the persisted, validated run because
the `go run` wrapper does not preserve exit codes above `1`. It does not launch
producers, deliver to a network endpoint, or change the Nublar run contract.

Run `make nublar-consumer-check` from the repository root to exercise the
failed, passed, missing-artifact, and malformed-envelope paths against
checked-in fixtures.
