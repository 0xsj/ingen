# A frozen oracle needs a language-neutral handoff shape

The frozen oracle is the boundary where contract review becomes executable
input. It must be consumable without reopening the contract source or knowing
Sorna's Go types, while remaining free of subject observations.

Sorna now publishes [`ingen.oracle/v1`](../../sorna/spec/ingen.oracle-v1.schema.json)
with the sealed contract reference, policy digest, and materialized cases. The
runtime already enforced this identity; the schema makes it explicit to other
SDKs and CI tooling.

The schema is structural. Runtime validation still owns unique case and rule
IDs, allowed strengths, valid lowercase digests, materialization bounds before
the artifact is created, and canonical JSON bytes when an oracle is loaded.
The important separation is:

```text
contract source -> frozen oracle cases -> subject observations
```

The oracle schema describes the middle artifact and intentionally has no place
for subject observations. Those belong to run and evidence artifacts, which
must remain independently verifiable after execution.
