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
