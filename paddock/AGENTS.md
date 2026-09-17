# Paddock agent guide

Use Paddock to inspect and enforce architecture boundaries. Treat it as an
evidence-producing tool: an agent may inspect the graph, propose a policy
change, and explain a finding, but must not silently change the architecture
contract.

## First pass

Run these from the Ingen repository root, substituting the subject and policy:

```sh
paddock map <subject> --policy <policy> --format json
paddock check <subject> --policy <policy> --format json > paddock-report.json
paddock explain paddock-report.json --status blocking --format json
```

`map` shows the discovered component vocabulary and graph-scale summary.
`check` evaluates the current policy. `explain` turns findings into
agent-readable context and remediation hints; it does not change the verdict.

If a sealed lock exists, use the lock-backed CI path instead:

```sh
paddock ci <subject> --policy-lock <policy-lock> \
  --output paddock-ci-result.json
paddock explain paddock-ci-result.json --format json
paddock ci validate --input paddock-ci-result.json
```

For a provider-neutral workflow, use the `handoff` mode in
[`examples/ci/paddock-gate.sh`](examples/ci/paddock-gate.sh). It preserves the
CI result and emits a separate explanation for the agent.

If a script uses `set -e`, do not invoke a finding-producing `check` or `ci`
command without capturing its status: exit `1` is an expected architecture
verdict and would otherwise stop the script before explanation. Prefer
`handoff`, or capture the status and continue to `explain` while preserving the
original `0`/`1` result.

## Interpret results

- Exit `0`: no blocking findings.
- Exit `1`: architecture findings are present; inspect the report and
  explanation before proposing a change.
- Exit `2`: invalid policy, unreadable source, adapter failure, or another
  evaluation error. This is not an architecture pass or fail.
- In a CI artifact, `passed`, `failed`, and `error` carry the authoritative
  evaluation status.
- An explanation is diagnostic context. It must never be used to convert a
  failing result into a pass.
- When `explain` reads a CI artifact, use its `provenance` block to correlate a
  filtered explanation with the artifact hash and policy/lock/graph references.
- When a CI artifact has `status: error`, run `paddock ci validate --input ...`
  to confirm the envelope and read the recorded diagnostic. Do not interpret
  an evaluation error as either an architecture pass or an architecture fail.

When multiple rules report the same dependency edge, keep the findings
separate but use `related_rules` to explain that they are duplicate signals,
not independent edges.

## Propose changes safely

Before changing a policy, read the current policy, lock, and
[`DECISIONS.md`](DECISIONS.md). Keep an agent-generated policy separate from
the approved policy:

```sh
paddock policy validate --policy proposed-paddock.yaml
paddock policy diff --before paddock.yaml --after proposed-paddock.yaml \
  --format json
paddock policy review --before paddock.yaml --after proposed-paddock.yaml \
  --cases paddock-policy-tests.yaml --output paddock-policy-review.json
```

Do not automatically run `policy seal`, add a waiver, or create a baseline.
Those actions change the architecture contract or accepted debt and require
explicit human approval. A generated draft from `paddock init` is descriptive
until it has been reviewed.

## Language adapters

For a language without a built-in adapter, consume or generate a
language-neutral graph through the adapter protocol. Preserve the graph and
adapter metadata with the CI artifact so findings can be reproduced. The graph
records the external executable and non-secret invocation digests, while raw
arguments are deliberately omitted. Read
[`ADAPTER-PROTOCOL.md`](ADAPTER-PROTOCOL.md) before diagnosing an adapter
failure; do not treat a missing or failed adapter as a clean graph.

When checking Go source in a restricted or ephemeral runner, configure
`GOCACHE` to a writable job-local directory before invoking Paddock. A Go
toolchain cache access failure is an evaluation error (exit `2`), not a clean
architecture result.

## Useful references

- [`README.md`](README.md): command reference and examples.
- [`POLICY-DESIGN.md`](POLICY-DESIGN.md): policy semantics and architecture
  patterns.
- [`COMPATIBILITY.md`](COMPATIBILITY.md): stable artifact and protocol
  contracts.
- [`examples/overwatch/README.md`](examples/overwatch/README.md): real backend
  blocking and UI passing handoff replays.
