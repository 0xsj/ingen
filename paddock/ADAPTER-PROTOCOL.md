# Paddock external graph adapter protocol

Paddock can consume a dependency graph produced by another executable. This is
the language-extension seam: a Rust, Java, Kotlin, Python, or repository-local
tool can own parsing and resolution while Paddock owns classification, policy
evaluation, findings, and CI verdicts.

## Process contract

Invoke an adapter explicitly with `paddock graph`:

```sh
paddock graph . \
  --policy paddock.yaml \
  --adapter ./tools/paddock-rust-adapter \
  --adapter-arg --workspace \
  --adapter-arg . \
  --format json > paddock-graph.json

paddock ci . \
  --policy-lock paddock.lock.json \
  --adapter ./tools/paddock-rust-adapter \
  --adapter-arg --workspace \
  --adapter-arg . \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

For a one-off inspection or check, the same adapter can be selected with
`paddock graph` or `paddock check`. `ci` requires `--graph-output` so the
adapter result remains a durable, hashable input. Existing graph files can
still be supplied with `--graph` when the adapter runs as a separate CI step.

The adapter process has a deliberately small contract:

- Paddock writes exactly one `paddock.graph-request/v1` JSON request to stdin.
- The adapter writes exactly one `paddock.graph/v1` JSON document to stdout.
- Diagnostics belong on stderr and are included in a Paddock error when the
  adapter fails or returns invalid JSON.
- Exit status `0` means the graph document was produced. Any non-zero status is
  an adapter error and makes the command fail with status `2`.
- Adapter arguments are passed as argument values. Paddock does not assemble a
  shell command, so spaces and shell metacharacters in an argument are not
  reinterpreted by Paddock.

The adapter is selected by the user or CI configuration. It is not discovered
or executed implicitly, which keeps the supply chain and execution boundary
visible in a build configuration.

## Request

The request has this shape:

```json
{
  "schema": "paddock.graph-request/v1",
  "language": "rust",
  "source_unit": "file",
  "root": "/workspace/service",
  "roots": ["src"],
  "include": ["src/**/*.rs"],
  "exclude": ["src/generated/**"],
  "required_capabilities": {
    "source_units": ["file"],
    "edge_kinds": []
  }
}
```

The machine-readable request contract is
[`spec/paddock.graph-request-v1.schema.json`](spec/paddock.graph-request-v1.schema.json).

`root` and `language` are required. `source_unit`, `roots`, `include`, and
`exclude` come from the policy when a policy is supplied. Include and exclude
patterns are relative slash-separated paths; adapters should use them to limit
discovery, and Paddock applies the same boundary to the returned graph as a
safety measure. `required_capabilities` is the negotiation surface; Paddock
currently requires a matching source unit when the policy declares one and
leaves edge-kind selection open for future rule vocabulary.

## Response

The response is the same graph shape emitted by Paddock's built-in adapters:

```json
{
  "schema": "paddock.graph/v1",
  "language": "rust",
  "source_unit": "file",
  "root": "/workspace/service",
  "roots": ["src"],
  "module_path": "example/service",
  "capabilities": {
    "source_units": ["file"],
    "edge_kinds": ["import"]
  },
  "package_count": 1,
  "edge_count": 0,
  "packages": [
    {"import_path": "example/service", "path": "src/service.rs"}
  ],
  "edges": []
}
```

The machine-readable response contract is
[`spec/paddock.graph-v1.schema.json`](spec/paddock.graph-v1.schema.json).

Paddock validates the schema, root, language, counts, package identities,
internal edge targets, edge kinds, and declared capabilities before policy
evaluation. The response language and source unit cannot contradict the
request.

## Adapter conformance check

Adapter authors can exercise this protocol without running a policy:

```sh
paddock adapter validate /workspace/service \
  --language rust \
  --unit file \
  --adapter ./tools/paddock-rust-adapter \
  --format json
```

The command sends a request, validates the returned graph, and prints either a
compact text summary or the normalized `paddock.graph/v1` document. A valid
adapter exits `0`; a process failure, malformed graph, language mismatch, or
capability mismatch exits `2`. With `--format json`, those failures are
reported as `paddock.adapter-validation/v1` diagnostics with stable error
codes. `graph --format json` uses the same diagnostic when an explicit adapter
fails; successful output remains `paddock.graph/v1`.

For repeatable coverage, define an adapter-test manifest:

```yaml
schema: paddock.adapter-tests/v1
adapter:
  executable: python3
  args: [./tools/paddock-adapter, --workspace, "{{root}}"]
cases:
  - name: rust-files
    root: fixtures/rust-service
    language: rust
    source_unit: file
    required_edge_kinds: [import]
    expect: pass
    package_count: 12
  - name: unsupported-mode
    root: fixtures/rust-service
    language: go
    source_unit: file
    expect: error
    error_contains: unsupported language
```

Run or preflight the cases with:

```sh
paddock adapter test validate --cases adapter-tests.yaml
paddock adapter test --cases adapter-tests.yaml \
  --output adapter-test-result.json \
  --ci-result adapter-conformance-ci-result.json \
  --format json
paddock adapter test verify --input adapter-test-result.json --files
paddock ci validate --input adapter-conformance-ci-result.json
```

Case roots are resolved relative to the manifest. `{{root}}` and
`{{manifest_dir}}` may be used in adapter arguments. A case can expect a valid
graph or an adapter error and can assert package/edge counts.
The saved result includes the manifest path and SHA-256; `--files` makes the
verification command recheck that the manifest has not changed.
The optional CI result wraps the detailed result as opaque `report` JSON and
uses `kind: adapter-conformance` for shared CI aggregation.

This protocol is intentionally graph-oriented rather than language-oriented.
Adding a language means implementing its adapter outside the Paddock rule
engine; it does not require changing policy evaluation. The graph document is
also hashed into the `ingen.ci-result/v1` artifact when supplied through
`--graph`, preserving the exact external input used by CI.

## Policy proposals with an external adapter

The same executable can participate in a policy proposal review. `policy diff`
compares policy semantics without graph analysis; `policy review` runs the
proposal's policy-test cases with the adapter arguments and records the result:

```sh
paddock policy review \
  --before paddock-before.yaml \
  --after paddock-proposal.yaml \
  --cases paddock-policy-tests.yaml \
  --adapter python3 \
  --adapter-arg ./tools/rust-graph-adapter.py \
  --adapter-arg --workspace \
  --adapter-arg . \
  --output paddock-policy-review.json \
  --format json
```

This keeps parsing and resolution outside Paddock while using the same
language-neutral diff, test, review, verification, and sealing workflow.
