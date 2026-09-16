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

Adapter conformance can run as an independent CI job. It does not require a
policy lock or source analysis result:

```sh
export PADDOCK_ADAPTER_TESTS=adapter-tests.yaml
export PADDOCK_ADAPTER_TEST_RESULT=paddock-adapter-test-result.json
export PADDOCK_ADAPTER_CI_RESULT=adapter-conformance-ci-result.json

sh paddock/examples/ci/paddock-gate.sh adapter-test
```

The helper preserves the adapter-test exit status (`0` for all expected cases,
`1` for a conformance mismatch, `2` for invalid input or an unverifiable
result) and verifies the saved manifest hash before returning. When
`PADDOCK_ADAPTER_CI_RESULT` is set, it also writes and validates a shared
`ingen.ci-result/v1` envelope.

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

The graph path is optional; when neither a graph nor an external adapter is
configured, Paddock uses its built-in adapter.

The gate can also invoke an external adapter itself and persist the graph as
part of the CI artifact inputs. Set `PADDOCK_ADAPTER` to the executable and,
when arguments are needed, set `PADDOCK_ADAPTER_ARGS_FILE` to a text file with
one argument per line:

```sh
export PADDOCK_ADAPTER=python3
export PADDOCK_ADAPTER_ARGS_FILE=paddock-adapter.args
export PADDOCK_GRAPH_OUTPUT=paddock-graph.json
```

For example, the conformance fixture can be configured with an args file whose
lines are:

```text
paddock/examples/adapter/conformance-adapter.py
--workspace
/path/to/workspace
```

With these variables, `gate` passes the adapter and repeated
`--adapter-arg` values to Paddock, then writes `PADDOCK_GRAPH_OUTPUT`. Set
either `PADDOCK_GRAPH` or an external adapter/profile, not both. When a
versioned profile already contains the executable and arguments, set
`PADDOCK_ADAPTER_CONFIG` instead; it cannot be combined with the executable
or args-file variables.

For a reviewed profile, set `PADDOCK_ADAPTER_PROFILE_SHA256` to the exact
lowercase SHA-256 of that file. The helper verifies the profile before any
workflow phase runs; profile drift exits `1`, while an invalid profile exits
`2`.

Alternatively, a CI job can let Paddock invoke the adapter and persist the
graph in one command:

```sh
paddock ci . --policy-lock paddock.lock.json \
  --adapter ./tools/paddock-language-adapter \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

The adapter-test and gate phases can run in the same provider-neutral job. The
Rust fixture demonstrates the complete sequence with a committed conformance
manifest and the same external adapter used for architecture enforcement:

```sh
export PADDOCK_POLICY=paddock/examples/rust-hexagonal.yaml
export PADDOCK_LOCK=paddock-rust-hexagonal.lock.json
export PADDOCK_SOURCE_ROOT=paddock/examples/services/rust-hexagonal/good
export PADDOCK_ADAPTER_CONFIG=paddock/examples/rust-use-adapter.yaml
export PADDOCK_ADAPTER_PROFILE_SHA256=$(shasum -a 256 "$PADDOCK_ADAPTER_CONFIG" | awk '{print $1}')
export PADDOCK_ADAPTER_TESTS=paddock/examples/rust-use-adapter-tests.yaml
export PADDOCK_ADAPTER_TEST_RESULT=paddock-rust-adapter-test-result.json
export PADDOCK_ADAPTER_CI_RESULT=paddock-rust-adapter-ci-result.json
export PADDOCK_GRAPH_OUTPUT=paddock-rust-graph.json
export PADDOCK_RESULT=paddock-rust-ci-result.json

sh paddock/examples/ci/paddock-gate.sh adapter-test
sh paddock/examples/ci/paddock-gate.sh seal
sh paddock/examples/ci/paddock-gate.sh verify
sh paddock/examples/ci/paddock-gate.sh gate
paddock ci validate --input "$PADDOCK_RESULT"
```

The conformance phase validates the adapter independently, including its
rejection of a non-Rust request. The gate phase then evaluates the sealed
hexagonal policy, persists the adapter-produced graph, and records its hash in
the shared CI artifact.

The gate always writes an `ingen.ci-result/v1` artifact when evaluation starts,
including failed architecture checks. Its exit code is `0` for a pass, `1` for
blocking findings, and `2` for an evaluation error.

The envelope is language-neutral. A coordinator can load Paddock's result with
the shared InGen CI-result contract and preserve `report` and `explanation` as
opaque producer-owned JSON. See [`../../../core/ciresult/testdata/`](../../../core/ciresult/testdata/)
for examples of the same contract carrying a non-Go architecture report.

For an agent handoff, `handoff` runs the same lock-backed gate, preserves its
exit code, and then emits the deterministic explanation from the saved CI
artifact. Set `PADDOCK_EXPLANATION_FORMAT=json` when stdout will be consumed by
another tool; set `PADDOCK_EXPLANATION_OUTPUT` to save it instead. Optional
`PADDOCK_EXPLANATION_RULE` and `PADDOCK_EXPLANATION_STATUS` narrow the handoff:

```sh
export PADDOCK_SOURCE_ROOT=.
export PADDOCK_RESULT=paddock-ci-result.json
export PADDOCK_EXPLANATION_FORMAT=json
export PADDOCK_EXPLANATION_OUTPUT=paddock-explanation.json
export PADDOCK_EXPLANATION_RULE=application-not-infrastructure
export PADDOCK_EXPLANATION_STATUS=blocking

sh paddock/examples/ci/paddock-gate.sh handoff
```

The command returns `0` for a passing gate, `1` for blocking findings, and `2`
for an evaluation or explanation error. It never seals a policy, changes the
source tree, or changes the CI verdict based on the explanation.
The same mode works when `PADDOCK_ADAPTER` is set; the adapter-produced graph
is retained in the CI artifact before the explanation is generated.
For a failing external adapter graph, the helper still returns `1` and the
filtered explanation identifies the violated rule; it does not treat adapter
language as a special case.

Paddock can validate a persisted envelope without rerunning the source check:

```sh
paddock ci validate --input paddock-ci-result.json
paddock ci validate --input external-ci-result.json --format json
```

This is an envelope-integrity check. It returns `0` for a valid passed or
failed artifact and `2` for malformed or unreadable input; it does not turn a
recorded `failed` status into a second analysis decision.

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

## GitHub Actions example

[`github-paddock-release.yml`](github-paddock-release.yml) is a non-active
provider example. Copy it into a repository's `.github/workflows/` directory
when GitHub Actions is the chosen publisher. A `paddock-v*` tag builds the
four supported archives, verifies `release-manifest.json`, uploads the bundle,
and publishes a GitHub release containing the archives, checksum file, and
manifest.

Ingen now has the same workflow active at
`.github/workflows/paddock-release.yml`; it remains dormant until a
`paddock-v*` tag is pushed.
