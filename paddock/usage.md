# Paddock live usage

This guide is the practical path for adding Paddock to a project. The normal
workflow is:

```text
discover -> draft -> review -> test -> approve -> seal -> enforce in CI
```

Paddock checks dependency structure. It does not decide what an architecture
should be, and an LLM does not become the authority merely because it creates
or explains a policy. The policy and sealed lock remain explicit, reviewable
inputs; the CI result remains deterministic.

## 1. Build or install Paddock

From the InGen repository, build the development binary:

```sh
make -C paddock build
export PADDOCK="$PWD/.artifacts/paddock/paddock"
"$PADDOCK" version
```

For a restricted runner, give the Go toolchain writable caches before running
the Go adapter:

```sh
export GOCACHE="$PWD/.cache/paddock-go-build"
export GOMODCACHE="$PWD/.cache/paddock-go-mod"
mkdir -p "$GOCACHE" "$GOMODCACHE"
```

You can use an installed `paddock` command instead of `PADDOCK`. The built-in
adapters currently support Go packages, TypeScript/JavaScript files, and
Python files. Other languages use the external adapter protocol described in
[`ADAPTER-PROTOCOL.md`](ADAPTER-PROTOCOL.md).

Git is optional. When the source root is inside a Git worktree, Paddock adds
the source revision and dirty-worktree identity to CI artifacts and init
summaries.

## 2. Choose the source root

Run Paddock against the smallest meaningful source root. For a service in a
larger repository, use the service directory rather than the whole monorepo
unless the policy intentionally describes workspace-wide boundaries.

Set paths once for the rest of the walkthrough:

```sh
export SOURCE_ROOT=./service
export POLICY=paddock.yaml
export LOCK=paddock.lock.json
```

Policy roots, test-case roots, and source paths should be interpreted from the
working directory where the commands run. Run the commands consistently from
the project root.

## 3. Inspect the dependency graph

Before authoring rules, inspect what the adapter actually sees. With a built-in
Go adapter:

```sh
"$PADDOCK" graph "$SOURCE_ROOT" \
  --language go \
  --unit package \
  --format json > paddock-graph.json
```

For TypeScript or Python, use `--language typescript --unit file` or
`--language python --unit file`. If a policy already exists, Paddock can take
the language, source unit, roots, and scope from it:

```sh
"$PADDOCK" graph "$SOURCE_ROOT" \
  --policy "$POLICY" \
  --format json > paddock-graph.json
```

The graph is `paddock.graph/v1`. It is useful for checking package/file
counts, import paths, unresolved imports, and whether an adapter is reporting
the boundaries you expected. A saved graph can later be supplied with
`--graph` to `check`, `map`, `init`, or `ci`.

## 4. Generate a conservative draft policy

Start with `init`. It groups source units, guesses component roles, and writes
rules as warnings. It is an inventory and review starting point, not an
architecture verdict.

For a layered Go service:

```sh
export DRAFT_POLICY=/tmp/paddock-draft.yaml
export INIT_SUMMARY=/tmp/paddock-init.json

"$PADDOCK" init "$SOURCE_ROOT" \
  --template layered \
  --output "$DRAFT_POLICY" \
  --format json > "$INIT_SUMMARY"
```

Available scaffold templates are:

- `layered` for Go, TypeScript, and Python;
- `hexagonal` for Go;
- `modular-monolith` for Go;
- `feature-sliced` for TypeScript;
- `cyclic` for Go and TypeScript.

If the source root is the repository root, keep the first draft output outside
that root when provenance matters. This prevents the newly generated policy
file itself from being included in the dirty-worktree snapshot.

Inspect the JSON summary before reviewing the draft. In addition to counts and
unclassified components, it can contain:

- `source_vcs`, identifying the source revision and dirty state;
- `graph_sha256`, identifying the normalized graph and source-unit
  configuration consumed by the draft;
- `review_required`, which is always `true` for generated drafts.

If the source changes, rerun `init` and compare these identities rather than
trusting old counts.

## 5. Review classification and component boundaries

Open the generated YAML and review every component match and label. Pay
particular attention to `role`, `context`, `layer`, `feature`, and `slice`
labels. Correct the vocabulary before tightening severities.

Use the component map to see the classified graph:

```sh
"$PADDOCK" map "$SOURCE_ROOT" \
  --policy "$DRAFT_POLICY" \
  --format json > paddock-component-map.json
```

The map reports package counts, cross-component edges, external dependencies,
and unresolved dependencies. When a templated component name resolves to
multiple label sets, JSON output includes an `identity` for each variant and
identity fields on dependency entries. This keeps bounded-context variants
visible without requiring every consumer to understand the extended fields.

Do not proceed to sealing while important source units remain unclassified or
when a component match is broader than the intended boundary.

## 6. Validate the policy

Validate policy shape and semantic rule options before running source analysis:

```sh
"$PADDOCK" policy validate --policy "$DRAFT_POLICY"
"$PADDOCK" policy validate --policy "$DRAFT_POLICY" --format json > paddock-policy.json
```

Text mode gives a concise summary. JSON mode emits the normalized policy when
valid, or a `paddock.policy-validation/v1` diagnostic and exit code `2` when
invalid.

Run the draft against the source:

```sh
"$PADDOCK" check "$SOURCE_ROOT" \
  --policy "$DRAFT_POLICY" \
  --format json > paddock-report.json
```

Warnings are useful during policy authoring because they expose likely
boundaries without blocking the draft workflow. Once the architecture is
reviewed, change only intentional rules to `error` severity.

## 7. Add policy test cases

Create a versioned `paddock.policy-tests/v1` manifest with small good,
violating, and—when useful—evaluation-error cases. Require specific rule IDs
for important failures so a broad failure cannot accidentally satisfy the
test.

Validate the manifest independently:

```sh
"$PADDOCK" policy test validate \
  --cases paddock-policy-tests.yaml
```

Run the policy cases:

```sh
"$PADDOCK" policy test \
  --policy "$DRAFT_POLICY" \
  --cases paddock-policy-tests.yaml \
  --format json > paddock-policy-test-result.json
```

The expected outcomes are `pass`, `fail`, and `error`. A case mismatch exits
`1`; invalid policy or manifest input exits `2`. Keep these fixtures small and
focused on architectural boundaries rather than application behavior.

## 8. Propose policy changes safely

Once an initial policy is under version control, treat later changes as
proposals. Keep the current policy unchanged while an agent or developer
prepares a candidate:

```sh
cp "$POLICY" proposed-paddock.yaml
# Edit proposed-paddock.yaml after reviewing the affected boundaries.

"$PADDOCK" policy diff \
  --before "$POLICY" \
  --after proposed-paddock.yaml \
  --format json > paddock-policy-diff.json
```

For a proposal with regression cases, create the durable review artifact:

```sh
"$PADDOCK" policy review \
  --before "$POLICY" \
  --after proposed-paddock.yaml \
  --cases paddock-policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json

"$PADDOCK" policy review verify \
  --input paddock-policy-review.json \
  --files
```

Review the normalized changes, raw and canonical hashes, test outcomes, and
required rule IDs. A policy review can fail with exit `1` when an expected case
does not match. It does not approve or apply the proposal.

## 9. Approve and seal the policy

Human or designated architecture-owner approval belongs between review and
sealing. After approval, make the approved policy the project policy and seal
that exact file:

```sh
"$PADDOCK" policy seal \
  --input "$POLICY" \
  --output "$LOCK"

"$PADDOCK" policy verify \
  --policy "$POLICY" \
  --lock "$LOCK"
```

Commit the policy and lock together. The lock binds both the exact policy file
and its canonical semantic representation. A formatting or comment change
requires a new seal, even when the semantic policy is unchanged.

Do not run `policy seal` automatically in ordinary CI. Sealing is an approval
boundary, not an analysis step.

## 10. Run the authoritative CI gate

The lock-backed gate is the normal CI command:

```sh
"$PADDOCK" ci "$SOURCE_ROOT" \
  --policy-lock "$LOCK" \
  --output paddock-ci-result.json
```

The gate writes an `ingen.ci-result/v1` artifact containing the authoritative
status, report, explanation, policy reference, lock reference, graph evidence
when retained, producer version, and optional source Git provenance.

For a previously generated graph:

```sh
"$PADDOCK" ci "$SOURCE_ROOT" \
  --policy-lock "$LOCK" \
  --graph paddock-graph.json \
  --output paddock-ci-result.json
```

For an external language adapter, either invoke it directly:

```sh
"$PADDOCK" ci "$SOURCE_ROOT" \
  --policy-lock "$LOCK" \
  --adapter ./tools/paddock-language-adapter \
  --adapter-arg --workspace \
  --adapter-arg "$SOURCE_ROOT" \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

Or use a reviewed adapter profile:

```sh
"$PADDOCK" adapter profile validate --input paddock-adapter.yaml
"$PADDOCK" ci "$SOURCE_ROOT" \
  --policy-lock "$LOCK" \
  --adapter-config paddock-adapter.yaml \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

The normal exit contract is:

- `0` — the gate passed;
- `1` — the gate ran and has active blocking findings;
- `2` — invalid input, incompatible evidence, adapter failure, or evaluation
  error.

Exit `1` is an architecture result. Exit `2` means the result could not be
evaluated and must not be treated as compliance.

## 11. Produce an agent handoff

Explain a report or CI artifact without changing it:

```sh
"$PADDOCK" explain paddock-ci-result.json \
  --format json > paddock-explanation.json
```

For a focused remediation handoff:

```sh
"$PADDOCK" explain paddock-ci-result.json \
  --rule application-not-infrastructure \
  --status blocking \
  --format json > paddock-explanation.json
```

The explanation includes the observed edge, matched rule, policy reason,
related rules, triage state, suggested actions, and CI/source provenance. The
filter narrows the evidence; it never changes the overall CI verdict.

To validate a saved CI artifact without rerunning analysis:

```sh
"$PADDOCK" ci validate --input paddock-ci-result.json
"$PADDOCK" ci validate --input paddock-ci-result.json --format json
```

A valid failed artifact returns `0` from `ci validate` because validation is
checking artifact integrity, not re-evaluating the architecture. A malformed
or unreadable artifact returns `2`. Valid error artifacts retain and display
their recorded evaluation diagnostic in text mode.

For a provider-neutral shell handoff, the repository includes
[`examples/ci/paddock-gate.sh`](examples/ci/paddock-gate.sh). Its `handoff`
mode runs the lock-backed gate, preserves the gate exit code, and then emits a
deterministic explanation. Set `PADDOCK_EXPLANATION_FORMAT=json` and
`PADDOCK_EXPLANATION_OUTPUT` when another tool will consume the explanation.

## 12. Handle existing findings with baselines or waivers

Use a baseline only to adopt an existing policy without allowing those known
findings to block immediately:

```sh
"$PADDOCK" baseline "$SOURCE_ROOT" \
  --policy "$POLICY" \
  --output paddock-baseline.json

"$PADDOCK" ci "$SOURCE_ROOT" \
  --policy-lock "$LOCK" \
  --baseline paddock-baseline.json \
  --output paddock-ci-result.json
```

Baselines are tied to the canonical policy hash and stable finding identities.
They do not hide new findings, and stale entries are reported for cleanup.

Use policy waivers for explicit, owned exceptions. Every waiver should include
a reason, owner, and expiration date. Active waivers remain visible but do not
block; expired waivers remain blocking.

## 13. Verify external adapters independently

Before depending on an external adapter in CI, validate its profile and run a
conformance manifest:

```sh
"$PADDOCK" adapter profile validate --input paddock-adapter.yaml
"$PADDOCK" adapter test validate --cases adapter-tests.yaml
"$PADDOCK" adapter test \
  --cases adapter-tests.yaml \
  --output paddock-adapter-test-result.json \
  --ci-result paddock-adapter-ci-result.json
"$PADDOCK" adapter test verify \
  --input paddock-adapter-test-result.json \
  --files
```

Conformance checks language and source-unit negotiation, graph shape,
capabilities, process failures, and expected negative cases. Keep the adapter
test result independent from the architecture gate so adapter failure is not
mistaken for an architecture finding.

## 14. Use Paddock with an LLM safely

An agent can use the structured outputs in this order:

1. Run `init --format json`, `graph --format json`, or `map --format json` to
   understand the source vocabulary and graph.
2. Propose or edit a policy, keeping the proposal separate from the approved
   policy and lock.
3. Run policy validation, policy tests, and `policy diff`.
4. Produce and verify a `policy review` artifact.
5. Ask an owner to approve the architectural interpretation.
6. Seal the approved policy and let CI evaluate the lock.
7. Use `explain` to inspect findings and suggest remediation.

The agent may classify, propose, compare, and explain. It must not silently
approve a policy, rewrite a sealed lock, convert a failed gate into a pass, or
replace the CI verdict with its own judgment.

## Troubleshooting

### The Go adapter fails before analysis

Set writable `GOCACHE` and, when needed, `GOMODCACHE`. In managed runners a
toolchain cache may be read-only. Paddock preserves this as an evaluation error
with exit `2` rather than treating the source as compliant.

### `init` refuses to overwrite a file

Choose a new output path or pass `--force` deliberately. For automated agent
workflows, a temporary draft path is safer than replacing an existing policy.

### The command exits `1`

Read the report or explanation. Exit `1` means Paddock evaluated the graph and
found active blocking architecture findings, or a policy test/review expected
outcome did not match.

### The command exits `2`

Check policy syntax and semantics, source-root paths, graph compatibility,
adapter capabilities, profile integrity, and toolchain cache permissions. A
valid error CI artifact may be inspected with `ci validate`, but it is not a
passing architecture result.

### A summary's counts do not match a later gate

Compare `source_vcs` and `graph_sha256` from the init summary with the source
and artifact provenance. Dirty worktrees can change between runs; regenerate
the draft or treat the old summary as stale instead of reasoning from counts
alone.

## Contracts and examples

Machine-readable contracts are in [`spec/`](spec/). The main examples include:

- [`examples/hexagonal.yaml`](examples/hexagonal.yaml);
- [`examples/layered.yaml`](examples/layered.yaml);
- [`examples/modular-monolith.yaml`](examples/modular-monolith.yaml);
- [`examples/feature-sliced-frontend.yaml`](examples/feature-sliced-frontend.yaml);
- [`examples/overwatch/README.md`](examples/overwatch/README.md);
- [`examples/ci/README.md`](examples/ci/README.md).

Run the Paddock test and vet checks before changing a contract or adapter:

```sh
GOCACHE="$PWD/.cache/paddock-go-build" go test ./paddock/...
GOCACHE="$PWD/.cache/paddock-go-build" go vet ./paddock/...
```
