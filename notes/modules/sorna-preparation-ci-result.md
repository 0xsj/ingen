# Provider preparation can cross the CI boundary before execution

The source provider now emits two related artifacts:

- `preparation.json`, using `ingen.mutation-preparation/v1`, contains the
  language-neutral preparation summary, changed source files, target
  resolution, and source/binary identities;
- a `mutation-preparation` `ingen.ci-result/v1` envelope retains that report
  and binds it to the exact provider manifest and plan bytes.

The envelope is produced with:

```sh
sorna mutation provider preparation preparation.json \
  --provider provider.yaml --format ci-result \
  --output mutation-preparation-ci-result.json
```

Sorna checks that the summary and provider agree on provider ID, exact plan
hash, optional semantic plan identity, variant command paths, source hashes,
and binary hashes. This makes the
preparation stage visible to Nublar without making Nublar understand Go ASTs or
any future provider language.

Preparation is not behavioral verification. A valid preparation result says
that a provider produced internally consistent artifacts; the mutation campaign
and its evidence still decide whether the contract detects the changed
behavior.

## Schema and load boundary

The preparation summary now has a published closed JSON Schema at
`ingen.mutation-preparation/v1`. It describes the summary, variants, source and
binary identities, edit provenance, and optional target-resolution record for a
consumer that does not import Sorna's Go packages.

`campaign.LoadPreparationBytes` is the runtime load boundary. It rejects
unknown fields and trailing JSON values before applying the semantic validator,
then requires the canonical JSON representation. JSON Schema expresses the
structural shape; runtime validation still owns sequence ordering, unique
mutation IDs, and the relationships checked against the provider and plan.

This keeps preparation reviewable across SDKs without allowing a coordinator to
reinterpret source-language mutation details.
