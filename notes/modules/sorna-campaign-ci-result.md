# Sorna mutation campaigns at the CI boundary

The campaign result is a producer-owned aggregate with mutation-specific
semantics: a killed mutation is a successful campaign entry, while a survivor
or inconclusive entry is evidence that the contract needs attention. Nublar
should not need to understand those semantics in order to collect the result.

Sorna therefore adapts a verified campaign result to the shared
`ingen.ci-result/v1` envelope:

```sh
sorna mutation verify campaign-result.json \
  --format ci-result --source-root . \
  --output mutation-campaign-ci-result.json
```

The envelope uses `kind: mutation-campaign`, preserves the complete
`ingen.mutation-campaign-result/v1` result in `report`, and binds the campaign
result and plan through exact SHA-256 input references. Its explanation has a
small `sorna.mutation-campaign-explanation/v1` shape containing the summary and
only the entries that did not pass. Each failed entry has a stable category,
and the explanation also counts categories for simple CI consumers:

- `contract-insensitive`: a survivor; the contract did not reject the mutant;
- `insufficient-observation`: the run could not decide the outcome;
- `invalid-mutant`, `equivalent-mutant`, and `execution-timeout`: explicit
  mutation classifications;
- `execution-error`: provider or campaign execution failed;
- `campaign-failure`: a future or otherwise unclassified failure.

The category is diagnostic. The envelope status and exit code remain the
orchestration contract.

The adapter verifies each referenced evidence bundle before emitting a
non-error envelope. This matters because a campaign result is an aggregate
pointer, not a replacement for the evidence bundles. If the result or an
evidence binding cannot be verified, Sorna emits `status: error` and exit code
`2` so a collector can distinguish integrity failure from a failed mutation
campaign.

Nublar only composes the resulting status. It does not decide whether a
survivor is acceptable or reinterpret the mutation summary.
