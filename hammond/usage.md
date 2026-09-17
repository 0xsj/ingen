# Hammond usage

This guide describes the supported solo workflow for Hammond in this
repository. Hammond is a local, file-backed governance tool. It records which
contract artifact is approved, who approved it, and how later contract
versions relate to earlier versions.

It does not execute the contract, run Sorna, provide a hosted registry, or
authenticate an external identity provider.

## 1. Prerequisites

Run commands from the repository root:

```sh
cd /Users/sj/Desktop/dev/builds/ingen
```

You need Go installed. The examples use `go run`, so no Hammond binary
installation is required.

The current solo fixtures use the local actor `solo`, mapped to the
`product-reviewer` role by:

- `hammond/examples/solo/review-authority-v1.json`
- `hammond/examples/solo/review-policy-v1.json`

Keep the authority, policy, contract artifacts, and record store inside one
private trusted filesystem. The authority fixture is intentionally unsigned
for this single-operator boundary.

## 2. Choose a record store

The default store for this workspace is:

```sh
STORE=.hammond/records
```

The `.hammond/` directory is gitignored. Hammond creates the store directory
when a command needs it. Do not commit or copy the store as if it were an
independently authenticated authority boundary.

For a fresh walkthrough that will not touch the current approved records, use
another ignored directory:

```sh
STORE=.hammond/usage-demo
```

## 3. Validate a contract record before registration

Every record binds a contract identity to the exact SHA-256 of its artifact.
Validate that record, contract, policy, and authority before registering:

```sh
go run ./hammond/cmd/hammond validate \
  --record hammond/examples/solo/record-v3.json
```

The current v3 artifact is:

```text
hammond/examples/document-pipeline/contract-v3.canonical.json
sha256: c8f7f9f675f6355f7eda3cc3ee2a3f6a84ee8fdf209ee7bd35defae16e528745
```

If the contract bytes change, create a new contract version and recalculate
the SHA-256. Do not edit an already-approved artifact in place.

## 4. Register a new contract version

Registration accepts only a record in `registered` state with its initial
`registered` event:

```sh
go run ./hammond/cmd/hammond register \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json
```

Registration is immutable for that contract identity. If the identity already
exists, inspect it instead of registering it again:

```sh
go run ./hammond/cmd/hammond show \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json
```

The repository's `.hammond/records` store already contains the v3 record, so
do not rerun the registration command against that store.

## 5. Open a review safely

Always obtain the current revision immediately before a mutation:

```sh
go run ./hammond/cmd/hammond revision \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json
```

Copy the returned `revision` value into the next command:

```sh
go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json \
  --event hammond/examples/solo/event-review-opened-v3.json \
  --if-revision <revision-from-previous-command>
```

The record should now be `in_review`. If the command reports a revision
conflict, reread the record, obtain a fresh revision, and decide whether the
new state is still the one you intend to change.

## 6. Record an approval

Obtain a new revision after opening the review:

```sh
go run ./hammond/cmd/hammond revision \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json
```

Append the approval using that fresh revision:

```sh
go run ./hammond/cmd/hammond append-event \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json \
  --event hammond/examples/solo/event-approved-v3.json \
  --if-revision <review-revision>
```

The approval event must use the exact contract artifact SHA-256, the active
review cycle ID, and an actor authorized by the referenced policy. A successful
approval changes the record to `approved`.

## 7. Inspect the registry and lineage

List all records in the store:

```sh
go run ./hammond/cmd/hammond list --store "$STORE"
```

Read one record:

```sh
go run ./hammond/cmd/hammond show \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json
```

Validate and print all contract lineage:

```sh
go run ./hammond/cmd/hammond lineage --store "$STORE"
```

The current repository-local store should show:

```text
document-pipeline/document-pipeline@2 state=superseded
  -> document-pipeline/document-pipeline@3
document-pipeline/document-pipeline@3 state=approved
```

Lineage validation reloads each record's referenced policy, so custom solo
policies remain enforced after process restarts.

## 8. Create a successor contract version

Create a successor only when the contract behavior actually changes:

1. Write a new canonical artifact, for example `contract-v4.canonical.json`.
2. Give it a new contract version and calculate its exact SHA-256.
3. Create a successor record in `registered` state with that artifact reference
   and the same policy reference, unless the approval policy intentionally
   changes too.
4. Run `validate` against the successor record.
5. Obtain the predecessor's current revision.
6. Create the amendment with the successor record.

Example:

```sh
go run ./hammond/cmd/hammond amend \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json \
  --successor hammond/examples/solo/record-v4.json \
  --event-id solo-v4-amend-001 \
  --actor solo \
  --at 2026-09-18T00:00:00Z \
  --kind additive \
  --reason "Describe the contract behavior changed in v4." \
  --if-revision <v3-revision>
```

The amendment publishes the independently registered successor and links it
from the predecessor. The supported amendment kinds are `clarifying`,
`additive`, `restrictive`, `corrective`, and `breaking`.

## 9. Approve and supersede the predecessor

Open and approve the successor using the same revision-safe sequence described
above. After the successor is approved, obtain a fresh predecessor revision and
supersede it:

```sh
go run ./hammond/cmd/hammond supersede \
  --store "$STORE" \
  --record hammond/examples/solo/record-v3.json \
  --successor hammond/examples/solo/record-v4.json \
  --event-id solo-v4-supersede-001 \
  --actor solo \
  --at 2026-09-18T00:02:00Z \
  --if-revision <v3-revision-after-amendment>
```

Run `lineage` afterward. The predecessor should be `superseded` and the
approved successor should remain the current version.

## 10. Back up and recover

Back up these items together:

- the `.hammond/records/` directory;
- the contract artifacts referenced by the records;
- the review policy artifact; and
- the local authority artifact.

Restoring only the record store is insufficient because records contain
digest-bound local references to their contract and policy artifacts. After a
restore, run `show`, `list`, and `lineage` before making new mutations.

## 11. Current limits and when to stop

This usage guide covers the single-operator local boundary. Stop and revisit
the trust model before:

- copying authority or records to another machine;
- adding a second operator;
- allowing an automated job to make approvals;
- requiring issuer attribution independent of filesystem access; or
- introducing hosted persistence or shared organization membership.

Those cases require signed authority artifacts and an explicit root/trust
bootstrap process. Hammond currently does not provide secure out-of-band root
delivery, a hosted API, hosted artifact retention, or automatic Sorna
approvals.
