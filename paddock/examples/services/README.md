# Paddock service fixtures

These are deliberately small Go repositories for testing Paddock's graph and
rule engine. They are subjects, not production templates.

Each architecture has a `good/` baseline and a `violating/` variant with one
deliberate structural defect:

| Fixture | Deliberate violation |
| --- | --- |
| `layered-go` | the domain imports a transport package |
| `hexagonal-go` | the domain imports `net/http` |
| `modular-monolith-go` | orders imports billing internals directly |

The fixtures should remain small enough that a reviewer can hold the whole
graph in their head. Once the analyzer exists, each policy example should be
run against its baseline and violating subject, and the expected rule ID should
be asserted.

Suggested future checks:

```sh
paddock check paddock/examples/services/layered-go/good \
  --policy paddock/examples/layered.yaml

paddock check paddock/examples/services/layered-go/violating \
  --policy paddock/examples/layered.yaml
```

