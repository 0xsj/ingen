# @0xsj/amber

Portable application-level provenance for TypeScript applications. The
package provides immutable provenance values, scoped contexts, HTTP and
messaging propagation, logging and tracing projections, and storage contracts.

## Install

```sh
npm install @0xsj/amber
```

## Quick start

The cross-language walkthrough, including Go, context, trust, storage, and
verification guidance, is in the [getting started guide](../docs/getting-started.md).
For application-owned backends and integrations, see the
[adapter authoring guide](../docs/adapter-authoring.md).
Custom storage implementations can import the reusable
`runProvenanceStoreContract` helper from `@0xsj/amber/testing` in their tests.

```ts
import {
  Provenance,
  ProvenanceContext,
  httpMiddleware,
  withOutgoingRequest,
} from "@0xsj/amber";

const root = Provenance.start();
const request = withOutgoingRequest(new Request("https://example.test"), root);
const handler = httpMiddleware(async (_request, context) => {
  const child = context.provenance?.child({ origin: "incoming" });
  return new Response(child ? "tracked" : "untracked");
}, "reject");

const response = await handler(request, ProvenanceContext.empty());
```

The wire format and incoming-data policy are defined in the repository's
[specification](https://github.com/0xsj/ingen/tree/main/amber/spec/v1.md). The
package is currently version `0.1.0` and evolves alongside the shared Go
implementation.

## Development

From the repository root:

```sh
make check          # static checks, tests, conformance, and package check
make examples       # runnable composition examples
```

The package exports its public entry point through `dist/index.js` and
`dist/index.d.ts`; test files and internal examples are not part of the
published package artifact. The package includes its MIT license and repository
metadata for downstream consumers.
