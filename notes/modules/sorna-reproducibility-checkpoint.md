# Fresh-workspace reproducibility has two identities

## Question

Can the complete Sorna document workflow be rerun from a fresh source
workspace and produce the same reviewable inputs and outcomes?

## Run

On 2026-09-15, the workflow was run with:

```sh
make nublar-aggregate-fresh
```

The Make target copied the current source tree into
`/private/tmp/ingen-workspace.V43xIL`, excluding `.git`, `.artifacts`, and
`.cache`, and ran the complete Nublar document workflow there. Because the
repository worktree contains unrelated uncommitted changes, this is a fresh
source snapshot rather than a clean Git checkout.

## Result

- `ingen.nublar-result/v1`: `passed`, exit code `0`.
- All four required checks passed: provider review, mutation preparation,
  behavioral verification, and mutation campaign.
- The frozen oracle contained 7 cases and the clean baseline passed all 7.
- All three catalogue mutations were killed: `status-200-create`,
  `remove-name-create`, and `unsupported-type-500`.

## Stable artifact identities

The run preserved these stable identities:

- contract `document-pipeline@2`: `6b40dfb15fa67f96c9f3bc79bc46206d45f6d44124197b344757499299e43445`;
- oracle artifact: `9deb58ce9c89573b4ce81c2720aee1df5f9c6e40968f2912fdcf39a959ea7f59`;
- oracle policy: `f302e33aa0ae8d4bf77320932e38864840359431708e44207e400f78ea6665cd`;
- compiled `status-200-create` variant: `d523fd02901111f271a6670e7411fa1c9bc2114f1492db73444c65d8213588ac`;
- compiled `remove-name-create` variant: `38f2eace719ade234191250e3198fb78ab786216d040d30a9b672b98d8ba923a`.

The current top-level workflow artifact hashes were:

| Artifact | SHA-256 |
| --- | --- |
| baseline CI result | `3d12ac4ca819dbd08978e58c32cd4d839a8dc89c4a4c29ae748de0c7fc96d003` |
| mutation plan | `c876683233102b59a00482ee2ad21a49055e71ea5d0367eaaa6ceb575c5ba1a8` |
| provider review CI result | `1f94c7a1b3fd06fc8417158803f6934628c4b3f763c8a07be73accbd394a3aaf` |
| preparation CI result | `e404f35efcaa1fafba9cd79745229fc45ec8d1222b76a631a3fbc548bafbcba4` |
| campaign result | `3fae3b878042fd6d667bd9674710dd822ee7305cd7a714cc509717e45669b77f` |
| campaign CI result | `603d55336cbc405d386ea0addb4296acd2d8e3abd8e34a7107641644a60abdda` |
| Nublar aggregate | `b57d458ae8beff11915c464f768e0087013a436f51167bdbb9d83f45ae0cef6c` |

## Finding

The mutation plan hash differed from the immediately preceding fresh run:

- previous plan: `4889a0fdee5d36466555ddf9190338a830c8c9c8683553e3aff5d4ad2a1b36eb`;
- current plan: `c876683233102b59a00482ee2ad21a49055e71ea5d0367eaaa6ceb575c5ba1a8`.

The byte comparison showed that the only difference was
`baseline.run_id`. This is generated per baseline execution, so the current
plan hash is an exact run-bound identity, not a stable identity for the
contract/catalogue/oracle combination. That distinction is useful: changing
the baseline run should not be silently treated as the same complete input.

## Design implication and implementation

Sorna now exposes two explicitly named values:

1. an exact plan artifact hash, including the selected baseline run; and
2. a stable semantic plan identity, excluding execution-specific fields such
   as `baseline.run_id`.

The second value is reported by generated providers, preparation summaries,
provider reviews, and campaign results. It is not used to replace strict exact
plan-byte binding, so the baseline provenance used by campaign execution is
unchanged.

## Next use

Before adding another mutation operator, rerun from a committed clean checkout
and compare both identities. The current fresh-workspace command remains the
repeatable alpha smoke test.

The identity implementation was then verified in
`/private/tmp/ingen-workspace.tUh1UW`. It emitted the exact plan hash
`3406110fe3b633e44b2bb657d8699ce00571ecc963e55f5f81f81f41520bb6d4` and the
semantic plan hash
`55f928c744949d3b907e21ba5357a8ecacc69f5727166867e5a46fb4de0e1050`.
The same semantic hash appeared in the generated provider manifest,
preparation summary, and campaign result; the strict workflow still passed
all four Nublar checks.

The final verification run used `/private/tmp/ingen-workspace.LoYHSv`. Its
exact plan hash was
`30d2b8c98f0171704369754a2d8c4fa47bfcee86bb56afc7e5d9b6604feacf37`, while
the semantic hash remained
`55f928c744949d3b907e21ba5357a8ecacc69f5727166867e5a46fb4de0e1050`.
`sorna mutation verify` independently recomputed both references before
emitting the passing campaign CI result.

## Three-mutation comparison

After adding the named unsupported-document error mutation, two more fresh
source snapshots were compared on 2026-09-15:

| Identity | `/private/tmp/ingen-workspace.4GfBOQ` | `/private/tmp/ingen-workspace.cwvGvg` |
| --- | --- | --- |
| exact plan hash | `1306b049118e3c426841614db449b547afbb4937e93abe66909dd4c1ecc6939e` | `cd791ab880bedea83de10b820799b59b2aaef8c3027862a80ec53d2664eef1dc` |
| semantic plan hash | `9bf6dd1fba758c893782e10c843e298543b58cfcffa559f2b8dc9bd2eab6d324` | `9bf6dd1fba758c893782e10c843e298543b58cfcffa559f2b8dc9bd2eab6d324` |
| oracle hash | `9deb58ce9c89573b4ce81c2720aee1df5f9c6e40968f2912fdcf39a959ea7f59` | `9deb58ce9c89573b4ce81c2720aee1df5f9c6e40968f2912fdcf39a959ea7f59` |

Both runs passed all four Nublar checks, killed all three mutations, and
produced identical clean and mutated subject binary hashes. The exact plan
hash changed as expected with the generated baseline run ID; the semantic
identity did not.

These are still fresh snapshots of the current worktree, not committed
checkouts. The committed-checkout comparison remains the point at which this
result can be treated as a release-level reproducibility claim.
