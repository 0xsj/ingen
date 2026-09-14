# Paddock service fixtures

These are deliberately small repositories for testing Paddock's graph and rule
engine. They are subjects, not production templates.

Each architecture has a `good/` baseline and a `violating/` variant with one
deliberate structural defect:

| Fixture | Deliberate violation |
| --- | --- |
| `layered-go` | the domain imports a transport package |
| `hexagonal-go` | the domain imports `net/http` |
| `modular-monolith-go` | orders imports billing internals directly |
| `cyclic-go` | alpha and beta import each other |
| `feature-sliced-ts` | shared code imports a feature and features cross-import |
| `python-hexagonal` | the domain imports a concrete adapter |

The fixtures should remain small enough that a reviewer can hold the whole
graph in their head. The cyclic subject intentionally does not compile as a Go
program; Paddock uses `go list -e` and source parsing to report its dependency
cycle anyway. Each policy example is exercised by the acceptance suite with an
expected rule ID.

Suggested future checks:

```sh
paddock check paddock/examples/services/layered-go/good \
  --policy paddock/examples/layered.yaml

paddock check paddock/examples/services/layered-go/violating \
  --policy paddock/examples/layered.yaml

paddock check paddock/examples/services/feature-sliced-ts/violating \
  --policy paddock/examples/feature-sliced-frontend.yaml

paddock check paddock/examples/services/python-hexagonal/violating \
  --policy paddock/examples/python-hexagonal.yaml
```
