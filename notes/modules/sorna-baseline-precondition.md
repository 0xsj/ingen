# A mutation result needs a passing baseline

Sorna previously allowed a mutation run to be invoked with only a mutation ID
and expected rule. That was enough to exercise classification, but it allowed a
mutated run to be interpreted even when the clean subject had never passed.

The CLI now requires `--baseline-evidence <directory>` whenever
`--mutation-id` is supplied. Before launching the candidate subject, Sorna:

1. verifies the baseline evidence checksums and semantic streams;
2. requires an unmutated baseline whose contract verdict is `pass`;
3. compares the contract identity and frozen oracle identity;
4. compares the oracle and managed-subject policy hashes; and
5. records the baseline evidence path and run ID in the candidate run.

This makes a `killed` result meaningful as a comparison against a known-good
subject, rather than merely a red run with a mutation label.

## Example

```sh
make sorna-run
go run ./sorna/cmd/sorna run \
  --oracle .artifacts/document-pipeline-oracle/oracle.json \
  --baseline-evidence .artifacts/document-pipeline-run \
  --mutation-id status-200-create \
  --expected-rule document.create.valid.accepted \
  ...
```

The baseline path is retained for audit and replay. Its contract, oracle, and
policy hashes are the comparison boundary; the subject URL and variant are
expected to differ.

## Limits

This is a precondition for one mutation run, not yet a mutation campaign
planner. Sorna still needs a catalogue and repeatable orchestration for larger
mutation sets.
