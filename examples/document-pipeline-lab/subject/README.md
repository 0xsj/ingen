# Document pipeline subject

This is the clean baseline implementation for the document-pipeline lab. It
uses Go's standard HTTP library and an in-memory store so that the first
verification run has no database, worker, network, or clock dependency.

The public behavior is defined by the draft contract in
[`../contract/contract.yaml`](../contract/contract.yaml). The subject exposes
the four HTTP/JSON entrypoints listed there and leaves ID formatting and the
internal queue implementation unspecified.

Run its tests with:

```sh
GOCACHE=/private/tmp/ingen-go-cache go test ./examples/document-pipeline-lab/subject/...
```

Run it locally with:

```sh
go run ./examples/document-pipeline-lab/subject/cmd/document-pipeline
```

This subject is a fixture for Sorna, not a production document processor.
