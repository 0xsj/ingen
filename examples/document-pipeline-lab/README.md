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
The clean Go subject now lives in [`subject/`](subject/). The oracle, defect
variants, and run artifacts will be added one small step at a time.

With the subject running in one terminal, the first Sorna runner can be
started from another:

```sh
make sorna-run
```

This runner evaluates all seven rules. Stateful rules use public setup requests
and captures to establish their preconditions before the target request.
