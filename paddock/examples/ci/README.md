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

The gate always writes an `ingen.ci-result/v1` artifact when evaluation starts,
including failed architecture checks. Its exit code is `0` for a pass, `1` for
blocking findings, and `2` for an evaluation error.

The `seal` phase should not run automatically in ordinary CI. A policy edit
must remain visible as a diff and require human approval before the replacement
lock is committed.
