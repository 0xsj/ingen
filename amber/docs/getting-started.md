# Getting started with Amber

Amber adds immutable application-level provenance to an existing Go or
TypeScript flow. Start with a root value, pass it through the application's
context boundary, and create a child when work crosses an application-owned
boundary.

Amber complements tracing and structured logging. It does not replace
authorization, authentication, or a transport's own delivery guarantees.

## Choose an SDK

For a released dependency, install the package for the runtime you use:

```sh
go get github.com/0xsj/ingen/amber
npm install @0xsj/amber
```

The Go and TypeScript SDKs implement the same v1 model and share conformance
fixtures. The package and module consumer checks in the repository demonstrate
the public boundaries before a release is published.

## Go

```go
package main

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
)

func main() {
	root, err := amber.Start()
	if err != nil {
		panic(err)
	}

	ctx, err := amber.WithProvenance(context.Background(), root)
	if err != nil {
		panic(err)
	}

	incoming, ok := amber.ProvenanceFromContext(ctx)
	if !ok {
		panic("missing provenance")
	}

	child, err := incoming.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		panic(err)
	}
	fmt.Println(child.ExecutionID())
}
```

For standard HTTP runtimes, use `adapters/http` to install incoming headers and
propagate the child on the response. For storage, choose the in-memory or file
store for local use, the generic key-value seam for an application-owned
backend, or the optional `adapters/storage/postgres` package for PostgreSQL.

## TypeScript

```ts
import { Provenance, ProvenanceContext } from "@0xsj/amber";

const root = Provenance.start();
const context = ProvenanceContext.empty().withProvenance(root);
const incoming = context.provenance;

if (!incoming) {
  throw new Error("missing provenance");
}

const child = incoming.child({ origin: "incoming" });
console.log(child.execution_id);
```

For Fetch-compatible HTTP runtimes, use `withIncomingRequest` at the inbound
boundary and `withOutgoingRequest` or `withOutgoingResponse` for propagation.
The TypeScript package also provides transport, messaging, logging, tracing,
storage, and optional OpenTelemetry adapters from its public entry point.

## Context and trust

Incoming values should be inspected before they are installed. Choose the
explicit `reject` or `ignore` policy for malformed input and provide an
application-owned trust validator when the deployment needs more than
structural validation. Amber v1 transport values are unsigned by default.

## Verification

From a checkout of the repository:

```sh
make test
make release-check
```

The runnable examples show HTTP, messaging, storage, logging, tracing, and
OpenTelemetry composition without requiring external services. PostgreSQL is
opt-in and must follow the migration-first sequence in
[`RELEASE.md`](../RELEASE.md).

## Further reading

- [`spec/v1.md`](../spec/v1.md) — core invariants, transitions, and wire shape
- [`spec/storage-v1.md`](../spec/storage-v1.md) — optional storage contract
- [`spec/trust-v1.md`](../spec/trust-v1.md) — structural and trust boundaries
- [`examples/README.md`](../examples/README.md) — runnable compositions
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — development workflow
- [`RELEASE.md`](../RELEASE.md) — release handoff checklist
