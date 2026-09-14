# Paddock CI workflow

[`paddock-gate.sh`](paddock-gate.sh) is a provider-neutral workflow example.
It separates policy proposal, approval, sealing, and ordinary CI evaluation.

## Workflow

An agent or developer may propose a policy change. Review the semantic and raw
input hashes before approving it:

```sh
export PADDOCK_POLICY=paddock.yaml
export PADDOCK_PROPOSED_POLICY=proposed-paddock.yaml
export PADDOCK_DIFF=paddock-policy-diff.json

sh paddock/examples/ci/paddock-gate.sh review
```

For a proposal with fixture expectations, use the durable review artifact:

```sh
export PADDOCK_CASES=paddock-policy-tests.yaml
export PADDOCK_REVIEW=paddock-policy-review.json

sh paddock/examples/ci/paddock-gate.sh review
```

This runs `paddock policy review`, verifies the saved artifact, and preserves
its exit code (`0` for passing tests, `1` for an expected-outcome mismatch).
Without `PADDOCK_CASES`, `review` retains the lightweight policy-diff behavior
and writes `PADDOCK_DIFF`.

After human approval, seal the approved policy and commit the lock artifact:

```sh
export PADDOCK_LOCK=paddock.lock.json

sh paddock/examples/ci/paddock-gate.sh seal
sh paddock/examples/ci/paddock-gate.sh verify
```

The regular CI job consumes only the lock. It does not need the mutable source
policy file:

```sh
export PADDOCK_SOURCE_ROOT=.
export PADDOCK_RESULT=paddock-ci-result.json

sh paddock/examples/ci/paddock-gate.sh gate
```

When the project uses an external language adapter, generate and retain the
graph first, then point the same gate at it:

```sh
export PADDOCK_GRAPH=paddock-graph.json
sh paddock/examples/ci/paddock-gate.sh gate
```

The graph path is optional; without it, Paddock uses its built-in adapter.

Alternatively, a CI job can let Paddock invoke the adapter and persist the
graph in one command:

```sh
paddock ci . --policy-lock paddock.lock.json \
  --adapter ./tools/paddock-language-adapter \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

The gate always writes an `ingen.ci-result/v1` artifact when evaluation starts,
including failed architecture checks. Its exit code is `0` for a pass, `1` for
blocking findings, and `2` for an evaluation error.

The `seal` phase should not run automatically in ordinary CI. A policy edit
must remain visible as a diff and require human approval before the replacement
lock is committed.

For a language that is not built into Paddock, generate the graph before the
gate and pass the exact file to CI:

```sh
paddock graph . --policy paddock.yaml \
  --adapter ./tools/paddock-language-adapter \
  --format json > paddock-graph.json
paddock ci . --policy-lock paddock.lock.json --graph paddock-graph.json \
  --output paddock-ci-result.json
```

See [`../../ADAPTER-PROTOCOL.md`](../../ADAPTER-PROTOCOL.md) for the request,
response, capability negotiation, and process failure contract.
