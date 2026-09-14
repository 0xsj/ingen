# Document pipeline lab

This is the first controlled Sorna subject. It is deliberately small, local,
and deterministic: a document is accepted, stored, processed through an
explicit public operation, and exposed through a public result.

The first version will use an HTTP/JSON boundary and a deterministic text or
Markdown processor. It will not depend on OCR, an LLM, a remote object store,
or wall-clock scheduling.

## What this lab is for

It should let us prove that Sorna can:

- validate and seal a contract;
- generate and freeze cases independently of the subject;
- observe a stateful public workflow;
- distinguish a clean baseline from deliberate defects;
- preserve rule-level results and evidence;
- teach the Go and testing concepts used to make those guarantees.

## Initial public workflow

```text
POST /documents
GET  /documents/{id}
POST /documents/{id}/process
GET  /documents/{id}/result
```

The current contract is a draft in [`contract/contract.yaml`](contract/contract.yaml).
The clean Go subject now lives in [`subject/`](subject/). The oracle is frozen
through the Sorna sandbox, while defect variants and run artifacts are added
one small step at a time.

The managed Sorna runner first freezes the oracle, builds the subject binary,
launches it under the separate subject policy, waits for its readiness
endpoint, runs from the frozen oracle, and tears the subject down:

```sh
make sorna-run
```

The subject exposes `GET /healthz` for lifecycle readiness; this endpoint is
operational plumbing, not one of the seven behavioral contract rules. Sorna
evaluates all seven rules. Stateful rules use public setup requests and captures
to establish their preconditions before the target request.

The first controlled defect can be exercised with:

```sh
make sorna-defect-run
```

The defect run is expected to be red, is labeled `status-200-create`, and
reports the mutation as `killed` in its run record.

The temporary fixture provider maps that defect binary to the mutation plan.
The complete first campaign can be exercised with:

```sh
make mutation-campaign-run
```

This creates one verified evidence bundle under
`.artifacts/document-pipeline-campaign/` and an aggregate
`campaign-result.json`. The prebuilt fixture remains available as a minimal
workflow proof. The first narrow source-level Go provider can be run
with:

```sh
make mutation-go-campaign-run
```

It copies the subject source, applies the reviewed status mutation with the Go
AST, and builds the variant without modifying the clean subject tree.
