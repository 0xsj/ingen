# Sorna contract-mutation inspection

## What this adds

Sorna now treats contract mutations as a distinct analysis plane. The command

```sh
sorna mutation contract inspect <catalogue> \
  --contract <draft-contract> \
  --oracle <frozen-oracle>
```

does not launch an implementation. It applies each `plane: contract` mutation
to a copied contract, seals the copy, regenerates an oracle with the original
policy identity, and compares the original and regenerated cases by rule ID.

## Concepts to keep track of

### Copy-on-write artifact handling

`ApplyContract` deep-copies the document before changing it. The source
contract remains unchanged, which matters because its hash is part of the
lineage. A sealed input is copied back to `draft` only inside the private
mutation copy so it can be sealed again as a new artifact.

### Contract mutation is not implementation mutation

An implementation mutation asks whether the frozen oracle detects a changed
subject. A contract mutation asks whether changing the authority changes the
oracle itself. Sending a contract mutation through the subject runner would
mix those questions and could create a misleading green result.

### Rule-ID comparison

Oracle case IDs are positional (`case-0001`, and so on), so removing an early
rule would shift every later case ID. The comparison therefore ignores case
position and compares cases by their stable `rule_id`. It reports changed,
removed, and added rule IDs.

### Visibility is not correctness

`visible` means the regenerated oracle changed. `equivalent` means it did not.
`invalid` means the declared mutation could not produce a valid contract/oracle
pair. None of these outcomes proves that the original contract is complete or
correct; they describe how observable the contract change is to Sorna's oracle
materialization boundary.

## Initial operators

Contract targets use the explicit `rule:<rule-id>` form:

- `contract.rule.remove`;
- `contract.rule.strength.replace`;
- `contract.rule.expect.status.replace`;
- `contract.rule.expect.required.remove`.

The report schema is `ingen.contract-mutation-result/v1`. It retains the
original and mutated artifact hashes so later CI or review tooling can bind to
the exact analysis inputs and outputs.

## CI handoff

The report can be adapted after inspection:

```sh
sorna mutation contract ci-result <report> \
  --contract <original-contract> \
  --oracle <frozen-oracle> \
  --output <ci-result>
```

The adapter reloads the report as canonical JSON, re-seals the supplied
contract, reloads the frozen oracle, and compares all three identities. A
report with invalid mutations is `failed`; an unreadable, non-canonical, or
mismatched input is `error`. The detailed report remains opaque inside the
shared `ingen.ci-result/v1` envelope, while the explanation lists visible and
invalid mutation IDs for CI consumers.
