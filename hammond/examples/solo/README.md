# Hammond solo approval example

This fixture uses a local authority snapshot. The actor `solo` is an example
stable local identity; replace it with the identity you want recorded in your
own workspace before using the files as operational configuration.

From the repository root:

```sh
STORE=/tmp/hammond-solo-records

go run ./hammond/cmd/hammond validate \
  --record hammond/examples/solo/record-v2.json

go run ./hammond/cmd/hammond register \
  --store "$STORE" \
  --record hammond/examples/solo/record-v2.json

go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/solo/record-v2.json \
  --event hammond/examples/solo/event-review-opened.json

go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/solo/record-v2.json \
  --event hammond/examples/solo/event-approved.json
```

The final record is approved using one local actor and one local role. No
GitHub organization, browser session, or external membership provider is
required. For concurrent-safe mutations, run `revision` after registration
and again after opening the review, then pass each returned `revision` value
to the corresponding `append-event --if-revision` command.

The repository-local `.hammond/records` store contains the exercised v2 -> v3
lineage. Version 3 adds an explicit `document_not_queued` conflict rule for
attempts to process a document that is no longer queued; its artifact digest
is `c8f7f9f675f6355f7eda3cc3ee2a3f6a84ee8fdf209ee7bd35defae16e528745`.
