# A mutation campaign result needs a closed, canonical handoff

The mutation campaign result is the coordinator-facing summary of many
independent Sorna runs. It is not a replacement for those evidence bundles:
each entry points to its evidence path and records the manifest and checksum
hashes that identify the bytes used for the classification.

Sorna publishes the aggregate as `ingen.mutation-campaign-result/v1`. The
schema is closed at the result, plan, entry, evidence, summary, and diagnosis
objects, while the existing runtime validator owns relationships that JSON
Schema cannot express conveniently: sequence ordering, unique mutation IDs,
counter consistency, status/outcome compatibility, and actionable diagnosis.

`campaign.LoadResult` is the read boundary. It rejects unknown fields and
trailing JSON values, validates the campaign semantics, and requires canonical
JSON bytes. This matters because a mutation result is often consumed later by
CI or a coordinator; silently accepting a producer extension or rewritten
representation could detach the summary from the exact campaign that ran.

## Boundary lesson

The aggregate can say that a mutation was killed, survived, inconclusive, or
invalid, but it cannot turn that label into proof of contract correctness. The
per-mutation evidence remains authoritative for observations, and the campaign
result only composes those verified references.

## Used in

- [`ingen.mutation-campaign-result-v1.schema.json`](../../sorna/spec/ingen.mutation-campaign-result-v1.schema.json)
- [`sorna/internal/campaign/result.go`](../../sorna/internal/campaign/result.go)
- [`sorna mutation verify`](../../sorna/cmd/sorna/)

## Related

- [`A killed mutation proves sensitivity, not correctness`](../concepts/a-killed-mutation-proves-sensitivity.md)
- [`A mutation campaign can cross the CI boundary without losing its semantics`](sorna-campaign-ci-result.md)
- [`A campaign executor must preserve one clean comparison per mutation`](sorna-campaign-execution.md)
