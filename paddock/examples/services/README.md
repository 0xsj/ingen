# Paddock service fixtures

These are deliberately small repositories for testing Paddock's graph and rule
engine. They are subjects, not production templates.

Each architecture has a `good/` baseline and a `violating/` variant with one
deliberate structural defect:

| Fixture | Deliberate violation |
| --- | --- |
| `layered-go` | the domain imports a transport package |
| `hexagonal-go` | the domain imports `net/http`; its focused proposal variant has application code import an outbound adapter |
| `modular-monolith-go` | orders imports billing internals directly |
| `cyclic-go` | alpha and beta import each other |
| `architecture-boundaries-go` | the good variant uses an approved shared-kernel value; the violating variant imports storage from the domain and infrastructure from the application |
| `feature-sliced-ts` | shared code imports a feature and features cross-import |
| `monorepo-ts` | a shared workspace package imports an orders domain package |
| `python-hexagonal` | the domain imports a concrete adapter; its focused proposal variant isolates that boundary without a cycle |

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

paddock policy test \
  --policy paddock/examples/monorepo-ts.yaml \
  --cases paddock/examples/monorepo-typescript.policy-tests.yaml

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
the domain and that all source packages belong to the declared component set.
The failing case requires `domain-is-pure`,
`application-not-infrastructure`, and `layers-point-inward`, so the fixture
guards both the policy intent and the explanation rule IDs.

The same fixture also covers a focused policy proposal. The before-policy
intentionally rejects the good service's shared-kernel value; the current
architecture-boundaries policy proposes allowing it while preserving the
application and domain boundaries:

```sh
paddock policy review \
  --before paddock/examples/architecture-boundaries-before-shared-kernel.yaml \
  --after paddock/examples/architecture-boundaries.yaml \
  --cases paddock/examples/architecture-boundaries-proposal.policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json
```

The acceptance suite expects this to remain a one-rule diff with two passing
policy-test cases. Review success is evidence for a proposal; it does not
create a policy lock.

The modular-monolith policy test covers the bounded-context case:

```sh
paddock policy test \
  --policy paddock/examples/modular-monolith.yaml \
  --cases paddock/examples/modular-monolith.policy-tests.yaml
```

It proves that public context APIs and shared-kernel code are allowed while
direct access to another context's internals is attributed to both the
privacy and mediation rules.

The same subjects cover a focused cross-context proposal. The before-policy
has no cross-context boundary rules; the current modular-monolith policy adds
public-API privacy and mediated-access enforcement:

```sh
paddock policy review \
  --before paddock/examples/modular-monolith-before-boundaries.yaml \
  --after paddock/examples/modular-monolith.yaml \
  --cases paddock/examples/modular-monolith-proposal.policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json
```

The acceptance suite expects two added rules, a passing good case, and a
violating case attributed to both boundary rules.

The hexagonal fixture also covers a focused adapter-direction proposal. Its
before-policy leaves application-to-adapter imports unregulated; the proposal
adds only `application-points-inward` and uses a deliberately small violating
service where application code imports an outbound adapter:

```sh
paddock policy review \
  --before paddock/examples/hexagonal-before-application-boundary.yaml \
  --after paddock/examples/hexagonal-application-boundary-proposal.yaml \
  --cases paddock/examples/hexagonal-application-boundary-proposal.policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json
```

The proposal must remain a one-rule diff, with the good service passing and
the violating service attributed to `application-points-inward`.

The TypeScript monorepo fixture verifies that the same proposal workflow is
language-neutral. The before-policy checks only graph integrity; the proposal
adds one `shared-is-independent` rule for the package alias boundary:

```sh
paddock policy review \
  --before paddock/examples/monorepo-typescript-before-shared-boundary.yaml \
  --after paddock/examples/monorepo-typescript-shared-boundary-proposal.yaml \
  --cases paddock/examples/monorepo-typescript-shared-boundary-proposal.policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json
```

The proposal must remain a one-rule diff, with the good workspace passing and
the violating alias import attributed to `shared-is-independent`.

The Python fixture completes the focused proposal matrix. Its before-policy
checks graph integrity and classification; the proposal adds only
`domain-is-pure`, while a deliberately small Python subject imports an adapter
from its domain through a relative import without introducing a cycle:

```sh
paddock policy review \
  --before paddock/examples/python-hexagonal-before-domain-purity.yaml \
  --after paddock/examples/python-hexagonal-domain-purity-proposal.yaml \
  --cases paddock/examples/python-hexagonal-domain-purity-proposal.policy-tests.yaml \
  --output paddock-policy-review.json \
  --format json
```

The proposal must remain a one-rule diff, with the good Python service passing
and the violating import attributed to `domain-is-pure`.

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
