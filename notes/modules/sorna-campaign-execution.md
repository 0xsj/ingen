# A campaign executor must preserve one clean comparison per mutation

Sorna now has a small provider/executor boundary. A provider manifest maps each
planned mutation ID to a prepared subject command. The campaign executor then
launches one fresh managed Sorna run per entry against the same frozen oracle
and passing baseline.

## Provider boundary

The first manifest uses `ingen.mutation-provider/v1` and keeps arguments as an
argv array. Only `${SORA_ADDR}` and `${SORA_URL}` are expanded. There is no
shell parsing, command-string interpolation, or implicit source edit permission.

This is deliberately a prebuilt fixture provider for the document lab. A
future Go, TypeScript, or other language provider can build or mutate a subject
as long as it returns the same prepared-command boundary and preserves the
plan's mutation identity.

## Execution behavior

For each plan entry Sorna:

1. verifies the plan, provider coverage, oracle, and policy hashes;
2. allocates a distinct local address and evidence directory;
3. passes every expected rule ID to the existing single-run path;
4. verifies the resulting evidence bundle and reads its mutation result;
5. hashes the verified bundle manifest and checksum file; and
6. records the outcome and those evidence hashes in `ingen.mutation-campaign-result/v1`.

Before verification, the executor attaches the exact plan and provider bytes
to the per-mutation bundle under `campaign/`. The bundle manifest records
their source paths and SHA-256 digests, and `checksums.sha256` covers both the
copied inputs and the provenance record.

The campaign continues after a `survived` or `inconclusive` entry so the
denominator remains visible. A killed mutation returns a successful campaign
entry even when its nested contract verdict is `fail`, because that red result
is the intended sensitivity signal.

Campaign output is rejected when it overlaps the baseline evidence directory,
and an existing per-mutation directory is not reused. These controls protect
the comparison artifact and avoid mixing observations across attempts.

## Commands

```sh
make mutation-provider-validate
make mutation-campaign-run
```

The document lab's provider maps `status-200-create` to the prebuilt defect
binary. This demonstrates the workflow without claiming that Sorna can yet
apply arbitrary source-level mutations.

## Limits

The per-mutation Sorna evidence bundles remain authoritative. The aggregate
result is still not a signed attestation, but each completed entry now binds
its evidence path to the SHA-256 of `manifest.json` and `checksums.sha256`.
The default provider is still a prebuilt fixture manifest, while the first
optional Go source provider covers one document-lab operator. Provenance
protects the exact inputs consumed by this executor, but it does not by itself
attest that a provider produced the declared variant honestly.

`sorna mutation verify` is the review-side check. It loads the canonical
campaign result, re-verifies each referenced evidence bundle, re-hashes its
manifest and checksum file, and rejects any mismatch with the aggregate
record.

## Used in

- [`sorna/internal/campaign`](../../sorna/internal/campaign/)
- [`sorna mutation run`](../../sorna/cmd/sorna/)
- [`document-pipeline provider`](../../examples/document-pipeline-lab/mutations/provider.yaml)

## Related

- [`A campaign plan freezes mutation inputs before execution`](sorna-campaign-plan.md)
- [`Mutation results separate target sensitivity from setup fallout`](sorna-mutation-result-model.md)
