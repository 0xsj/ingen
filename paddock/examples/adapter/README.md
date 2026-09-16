# Adapter fixture

`fixture-adapter.sh` is a pass-through executable for testing the Paddock
adapter boundary. It consumes the JSON request from stdin and returns a graph
document supplied as its first argument. It deliberately does not parse source
code; real adapters own language parsing and resolution.

Example:

```sh
sh paddock/examples/adapter/fixture-adapter.sh paddock-graph.json \
  | paddock graph . --input /dev/stdin --format json
```

For direct checking or CI, pass the fixture as an adapter argument:

```sh
paddock check . --policy paddock.yaml \
  --adapter sh \
  --adapter-arg paddock/examples/adapter/fixture-adapter.sh \
  --adapter-arg paddock-graph.json

paddock ci . --policy-lock paddock.lock.json \
  --adapter sh \
  --adapter-arg paddock/examples/adapter/fixture-adapter.sh \
  --adapter-arg paddock-graph.json \
  --graph-output paddock-graph.json \
  --output paddock-ci-result.json
```

The machine-readable contracts are
[`paddock.graph-request-v1.schema.json`](../../spec/paddock.graph-request-v1.schema.json)
and [`paddock.graph-v1.schema.json`](../../spec/paddock.graph-v1.schema.json).

## Conformance fixture

`conformance-adapter.py` is a dependency-free Python adapter that validates
the request and returns a small synthetic Rust graph. It is useful for testing
the external-language seam without requiring a Rust parser:

```sh
python3 paddock/examples/adapter/conformance-adapter.py \
  --workspace /path/to/workspace \
  < request.json

paddock graph /path/to/workspace \
  --policy paddock.yaml \
  --adapter python3 \
  --adapter-arg paddock/examples/adapter/conformance-adapter.py \
  --adapter-arg --workspace \
  --adapter-arg /path/to/workspace \
  --format json
```

The fixture supports `rust` with `file` source units and declares the
`import` edge kind. It identifies itself as
`paddock-conformance-python` version `1.0.0`, so the resulting graph and CI
handoff also demonstrate adapter-supplied provenance. It intentionally checks
capability negotiation and that adapter arguments preserve the workspace
boundary.
Pass `--violate` after the workspace argument to emit a deliberate
domain-to-application edge for negative policy-enforcement tests.

## Real parser example

`python-ast-adapter.py` is a small dependency-free adapter that uses Python's
standard-library `ast` module to discover `.py` files and resolve local
absolute/relative imports. It is intentionally limited, but demonstrates the
shape of a real adapter: parsing and resolution stay outside Paddock, while
the emitted graph remains language-neutral.

Run it against the existing Python hexagonal fixtures:

```sh
adapter=paddock/examples/adapter/python-ast-adapter.py
source=paddock/examples/services/python-hexagonal/good

paddock adapter validate "$source" \
  --language python \
  --unit file \
  --adapter python3 \
  --adapter-arg "$adapter" \
  --adapter-arg --workspace \
  --adapter-arg "$PWD/$source"

paddock check "$source" \
  --policy paddock/examples/python-hexagonal.yaml \
  --adapter python3 \
  --adapter-arg "$adapter" \
  --adapter-arg --workspace \
  --adapter-arg "$PWD/$source"
```

The good fixture should pass. Repeat the command with
`python-hexagonal/violating` to see the adapter preserve the domain-to-adapter
edge for Paddock's `domain-is-pure` rule; that check should exit `1`. The adapter reports itself as
`paddock-python-ast` version `1.0.0`, so CI graph evidence also demonstrates
adapter-supplied identity.

For a graph-only inspection, `graph --language python` selects the default
`file` source unit automatically; pass `--unit` when an adapter needs a
different unit.

## Unsupported-language example

`rust-use-adapter.py` demonstrates the same seam for a language without a
built-in Paddock adapter. It intentionally supports only ordinary Rust `use`
and `mod` declarations, but it performs real source discovery and import
resolution rather than returning a fixed graph:

```sh
adapter=paddock/examples/adapter/rust-use-adapter.py
policy=paddock/examples/rust-hexagonal.yaml
good=paddock/examples/services/rust-hexagonal/good

paddock adapter validate "$good" \
  --language rust \
  --unit file \
  --adapter python3 \
  --adapter-arg "$adapter" \
  --adapter-arg --workspace \
  --adapter-arg "$PWD/$good"

paddock check "$good" \
  --policy "$policy" \
  --adapter python3 \
  --adapter-arg "$adapter" \
  --adapter-arg --workspace \
  --adapter-arg "$PWD/$good"
```

The good subject should pass. Run the same check against
`rust-hexagonal/violating` to produce the domain-to-adapter finding. No Rust
toolchain is required for this fixture; a production adapter can later replace
the example parser while preserving the Paddock protocol.

For repeated commands, the checked-in profile removes the repeated executable
and workspace arguments:

```sh
paddock check "$good" \
  --policy "$policy" \
  --adapter-config paddock/examples/rust-use-adapter.yaml
```

The committed adapter-test manifest makes the adapter contract repeatable:

```sh
paddock adapter test validate \
  --cases paddock/examples/rust-use-adapter-tests.yaml

paddock adapter test \
  --cases paddock/examples/rust-use-adapter-tests.yaml \
  --output paddock-rust-adapter-test-result.json \
  --format json
```

It checks both fixture graph shapes and confirms that a non-Rust request is
rejected before Paddock evaluates any architecture policy.

An adapter-test manifest may use the same reusable profile as the architecture
gate:

```yaml
schema: paddock.adapter-tests/v1
adapter:
  profile: rust-use-adapter.yaml
```

The profile path is resolved relative to the manifest. A manifest must choose
either `adapter.profile` or `adapter.executable`; profile-backed results retain
the exact profile file reference.

Run the adapter conformance check directly when developing an adapter:

```sh
paddock adapter validate /path/to/workspace \
  --language rust \
  --unit file \
  --adapter python3 \
  --adapter-arg paddock/examples/adapter/conformance-adapter.py \
  --adapter-arg --workspace \
  --adapter-arg /path/to/workspace
```

For repeatable adapter coverage, use an adapter-test manifest and persist its
evidence for CI:

```sh
paddock adapter test --cases adapter-tests.yaml \
  --output adapter-test-result.json \
  --ci-result adapter-conformance-ci-result.json \
  --format json
paddock adapter test verify --input adapter-test-result.json --files
paddock ci validate --input adapter-conformance-ci-result.json
```
