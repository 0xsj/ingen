# InGen Core

Placeholder for the shared, language-neutral artifact and protocol layer.

It will define the contracts, run specifications, rule results, mutation
records, lifecycle events, evidence manifests, and compatibility versions used
by Sorna, Sentinel, CI, and future SDKs.

This area should not contain the verification engine itself. Its purpose is to
make different InGen surfaces interoperable.

The first shared result contract is described in
[`CI-RESULT-SPEC.md`](CI-RESULT-SPEC.md). Producers such as Paddock own their
nested report semantics; consumers such as Nublar use the envelope's status,
exit code, input references, and artifact schemas without duplicating the
verification engine.

The Go representation lives in [`ciresult/`](ciresult/). It validates only the
shared envelope and intentionally keeps producer reports as raw JSON.
