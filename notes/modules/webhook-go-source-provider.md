# A source provider should share preparation mechanics but own target meaning

The webhook source-provider slice is useful only when its generic copy/build
machinery is shared while target selection remains explicit for the webhook
subject.

## Origin

The first webhook mutation used a prebuilt wrapper. That proved the Sorna
campaign boundary, but left the language-level provider boundary untested for a
second vertical.

## What

The reusable Go provider now accepts a subject mutation set. The webhook set
parses `receiveEvent` in a copied source tree, selects the single duplicate
guard, and changes its condition from `exists` to `exists && false`. The
provider records
the changed source tree, binary, exact plan hash, semantic plan identity, and
target-resolution counts before publishing the manifest.

## Why

The provider should not infer webhook semantics from generic AST machinery, and
the webhook mutator should not reimplement copying, building, hashing, or
manifest publication. Keeping those responsibilities separate lets another
vertical reuse the provider without pretending that a source pattern is a
universal mutation operator.

## Gotchas

- The mutator receives only the copied source root, so preparation cannot write
  back into the clean source tree.
- Exactly one duplicate guard must be selected. Missing or ambiguous guards
  produce a typed resolution error and no source write.
- The managed subject can read the prepared binary directory but must not read
  the copied source-provider variants.
- Exact plan binding is required for this source-level path even though the
  earlier fixture provider remains optional-binding for local demonstrations.

## Result

The strict webhook source-provider campaign passed through
`make webhook-go-mutation-alpha`. The baseline passed all 4 rules; the source
variant killed 1 mutation with 3 rules passing and
`webhook.duplicate-is-idempotent` failing. The provider review and preparation
envelopes both passed with exact plan binding. Measured identities from the
fresh workspace were:

- exact plan: `205ad36dbd26e1e5260d60c21d2b1a896937be3ac5e320ab09d5d1eddbd5f9c2`;
- semantic plan: `d4f849ff81f9cec9f05d416d7e70801eff2fd4933cc21384c18ac8e83f97b741`;
- provider manifest: `5d3590f3ee26e8d45126138caf77c5fd3f5d51ea7889ba08facbc3c42095b2e4`;
- preparation summary: `37f4faea43f638d3425d2cf0e057981bfe9bb046bd9c5cca3851e5e0107d6edc`;
- provider review CI result: `c8636b1a63e4886159719f4cc550b63c63314d8a2828e004c9b7d0e1928d788f`;
- preparation CI result: `6bf2f9bb6b39cc60f910cb8488d90f0a507f59dd501c0529de454247d43d4a3a`;
- campaign result: `69c6b1d3e429d4e97c02f7886ed6eed15a86249037461e4f429697b49a3b341d`;
- campaign CI result: `aee2cb9302df7fcdd3fad749c3fba42444e6f42d33b42c0b6684ca02b28b8a0f`;
- mutated binary: `efb859ec3db6268d7a23e64d837f81cbd9538a881509b70894fc75d4aff9dae1`.

## Used in

- `sorna/cmd/sorna-go-provider`
- `examples/webhook-validation-lab/mutations/catalogue.yaml`
- `make webhook-go-mutation-alpha`

## Related

- [`sorna-golang-provider.md`](sorna-golang-provider.md)
- [`sorna-provider-review.md`](sorna-provider-review.md)
- [`sorna-preparation-ci-result.md`](sorna-preparation-ci-result.md)
- [`sorna-subject-isolation.md`](sorna-subject-isolation.md)
