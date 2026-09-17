# Sorna provider handoff is frozen for the alpha

The provider conformance slice exposed the remaining interface question: the
manifest and review code worked, but the compatibility promise was implicit.
The alpha boundary is now written down in
[`sorna/PROVIDER-HANDOFF.md`](../../sorna/PROVIDER-HANDOFF.md).

The important decision is that Sorna freezes the handoff, not the provider
implementation. A future SDK may use a different language or build strategy
as long as it emits `ingen.mutation-provider/v1` and satisfies the same exact
capability, entry, plan-binding, provenance, and no-execution review rules.

The conformance corpus is the acceptance surface. It deliberately distinguishes
parseable manifests, complete handoffs, exact-plan drift, semantic-plan drift,
unsupported capabilities, partial preparation, and malformed entries. This
keeps a green local provider review from being confused with behavioral
mutation-killing evidence.

The freeze also records a compatibility rule: the closed v1 schema cannot gain
new fields or changed meanings silently. A breaking handoff requires v2. This
gives SDK work a stable target and prevents language-specific concerns from
leaking into Sorna's campaign executor.

Acceptance command:

```sh
make mutation-provider-conformance
```

The full Sorna checkpoint remains:

```sh
make sorna-alpha-check
```
