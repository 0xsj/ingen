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

To exercise replay against a fresh, separate clean subject, use:

```sh
make sorna-replay-fresh
```

This target produces the stored baseline first, starts a new subject through
the fixture harness in [`replay/`](replay/), waits for `GET /healthz`, and
writes a `behavioral-replay` CI result. The harness owns process lifecycle;
Sorna replay itself remains a verifier of an already-running URL.

The negative replay regression uses the controlled `unsupported-type-500` defect
and succeeds only when replay correctly reports a failed CI result:

```sh
make sorna-replay-defect-fresh
```

This keeps a deliberate red separate from infrastructure failure: the Make
target expects the replay command's failure exit code and preserves the full
report at `.artifacts/document-pipeline-replay-defect-ci-result.json`.

The two replay classifications can be exercised together with:

```sh
make sorna-replay-regression
```

The stateful `status-200-create`, `process-stays-queued`, and
`persistence-wrong-key` cases are expected to produce CI `error` with nested
`inconclusive` replay reports, because their broken setup or state prevents
later cases from being evaluated. The complete `unsupported-type-500`,
`remove-name-create`, and `accepts-png` cases are expected to produce CI
`failed` with nested `drifted` replay reports.

To turn that six-case regression run into one reviewable Sorna envelope, use:

```sh
make sorna-replay-matrix-ci-result
```

It writes `.artifacts/document-pipeline-replay-matrix-ci-result.json` with
the expected classifications and hashes for all six member results.

To verify that aggregate later, use:

```sh
make sorna-replay-matrix-verify
```

The temporary fixture provider maps the three prebuilt defect binaries to the
mutation plan. The complete fixture campaign can be exercised with:

```sh
make mutation-campaign-run
```

This creates one verified evidence bundle per mutation under
`.artifacts/document-pipeline-campaign/` and an aggregate
`campaign-result.json`. The prebuilt fixture remains available as a minimal
workflow proof. The narrow source-level Go provider can be run
with:

```sh
make mutation-go-campaign-run
```

It copies the subject source, applies a reviewed success-path, error-path, or
state-transition mutation with the Go AST, and builds each variant without
modifying the clean subject tree. The current catalogue covers create-response
status replacement, required-field removal, the unsupported-document error
status, a process-transition defect that leaves the public process result
queued, a persistence-key defect that makes the returned document ID
unreadable, and an input-validation defect that accepts PNG documents.

The opt-in survivor diagnostic is now a regression for the contract gap that it
previously exposed:

```sh
make mutation-go-survivor-ci-result-fresh
```

It adds an observable `debug: mutation` field to the create response. The
contract now sets `additional_properties: false`, so the campaign should exit
with a passing CI result and record the expected rule as `fail` in its
diagnosis. Before that contract amendment, the same campaign produced a
survivor with the expected rule recorded as `pass`.
