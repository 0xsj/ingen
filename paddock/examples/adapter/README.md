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
