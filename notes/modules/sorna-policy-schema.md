# The capability policy needs the same cross-language boundary as the contract

After the contract schema was published, the next Sorna-owned input was the
capability policy. A policy is the artifact that declares what the oracle or
subject role may read, write, connect to, and invoke, so consumers need a
stable shape before they can safely produce or review one in another SDK.

Sorna now owns the `ingen.policy/v1` identity and rejects legacy or arbitrary
schema values. The published
[`ingen.policy-v1.schema.json`](../../sorna/spec/ingen.policy-v1.schema.json)
describes the policy wrapper, filesystem rules, network modes and allowlist,
and process/tool capabilities.

The schema is structural and closed at the v1 policy boundary. Runtime
validation still owns semantic checks such as overlapping filesystem paths,
mode-dependent network entries, duplicate tool names, subject identity, and
draft-only sealing. The important invariant is the same as for contracts:

```text
published schema identity == runtime schema identity == example/spec identity
```

This keeps a policy produced by a future SDK on the same identity boundary as
the Go implementation without moving enforcement semantics into the schema.
