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
| `architecture-boundaries-go` | the good variant uses an approved shared-kernel value; the violating variant imports storage from the domain and infrastructure from the application |
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

Policy proposals can be regression-tested as a group:

```sh
paddock policy test \
  --policy paddock/examples/hexagonal.yaml \
  --cases paddock/examples/hexagonal.policy-tests.yaml
```

The architecture-boundaries fixture demonstrates the small boundary loop:

```sh
paddock policy test \
  --policy paddock/examples/architecture-boundaries.yaml \
  --cases paddock/examples/architecture-boundaries.policy-tests.yaml
```

The passing case proves that an approved shared-kernel package is usable from
the domain. The failing case requires `domain-is-pure`,
`application-not-infrastructure`, and `layers-point-inward`, so the fixture
guards both the policy intent and the explanation rule IDs.

The modular-monolith policy test covers the bounded-context case:

```sh
paddock policy test \
  --policy paddock/examples/modular-monolith.yaml \
  --cases paddock/examples/modular-monolith.policy-tests.yaml
```

It proves that public context APIs and shared-kernel code are allowed while
direct access to another context's internals is attributed to both the
privacy and mediation rules.

The same manifest-driven workflow is covered for the TypeScript and Python
adapters:

```sh
paddock policy test \
  --policy paddock/examples/feature-sliced-frontend.yaml \
  --cases paddock/examples/feature-sliced.policy-tests.yaml

paddock policy test \
  --policy paddock/examples/python-hexagonal.yaml \
  --cases paddock/examples/python-hexagonal.policy-tests.yaml
```

These cases verify that the same policy-test contract reports language-specific
graphs while preserving stable architecture rule IDs.

Layered direction and cycle detection are covered by the remaining Go
manifests:

```sh
paddock policy test \
  --policy paddock/examples/layered.yaml \
  --cases paddock/examples/layered.policy-tests.yaml

paddock policy test \
  --policy paddock/examples/cyclic.yaml \
  --cases paddock/examples/cyclic.policy-tests.yaml
```

The cyclic subject intentionally does not compile; its manifest preserves the
expected `no-cycles` result from Paddock's source-graph handling.

The manifest uses `paddock.policy-tests/v1`. Each case names a source root and
expects `pass`, `fail`, or `error`; roots are relative to the manifest file.
Negative cases may add `require_rules` to assert which rule IDs must explain
the violation, preventing a weakened policy from passing for the wrong reason.
