# A campaign plan freezes mutation inputs before execution

Sorna now turns the validated mutation catalogue into a canonical
`ingen.mutation-plan/v1` artifact. The plan is the handoff between review and a
future mutation provider.

## What the plan binds

The planner requires and records:

- the exact catalogue bytes and SHA-256 hash;
- the contract ID, version, and sealed hash from the frozen oracle;
- the oracle artifact hash;
- a checksum-verified passing baseline run and its run ID;
- oracle and managed-subject policy hashes;
- explicit sequence numbers for mutation execution order.

The catalogue is checked against the contract before planning. The baseline is
checked with the existing baseline precondition, so a plan cannot be created
from a failed or mutated clean run.

## Why planning is separate from applying

The catalogue describes what a mutation means; a provider will eventually
decide how to apply it for a language or build system. Separating those steps
keeps source-editing permissions explicit and makes the campaign inputs
reviewable before a working tree or subject process is touched.

## Commands

```sh
make mutation-plan
```

This writes `.artifacts/document-pipeline-mutation-plan.json`. The plan is
canonical JSON and includes a plan hash in the CLI output.

## Limits

This slice only constructs the immutable handoff. It does not choose a
mutation provider, apply patches, launch one fresh subject per entry, or append
campaign inputs/results to the run evidence bundle; those responsibilities
belong to the later campaign executor and language-specific provider.

## Used in

- [`sorna/internal/campaign`](../../sorna/internal/campaign/)
- [`sorna/internal/mutation`](../../sorna/internal/mutation/)
- [`document-pipeline catalogue`](../../examples/document-pipeline-lab/mutations/catalogue.yaml)

## Related

- [`A mutation catalogue makes the experiment reviewable before execution`](sorna-mutation-catalogue.md)
- [`A mutation result needs a passing baseline`](sorna-baseline-precondition.md)
