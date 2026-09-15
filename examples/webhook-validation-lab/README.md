# Webhook validation lab

This is the second Sorna subject: a small webhook ingestion boundary used to
prove that the contract remains useful outside document processing.

The first slice covers three externally visible behaviors:

- a supported event is accepted with `202`;
- malformed or unsupported events are rejected with explicit error codes;
- repeating an accepted event ID is idempotent and returns `200` with a
  duplicate status.

The subject is deliberately stateful, but its state is only an in-memory set of
accepted event IDs. There is no signature scheme, queue, or delivery retry
model. Those should be added only when the contract needs to make their
semantics explicit.

Run the baseline with:

```sh
make webhook-alpha
```

The command validates the contract and policies, runs the subject tests, freezes
the oracle, launches a managed subject, and emits a Sorna CI result.

Run the first fixture-based mutation campaign with:

```sh
make webhook-mutation-alpha
```

It disables duplicate idempotency in one controlled variant and checks that the
duplicate rule catches it. This is a campaign boundary proof; it is not yet a
webhook-specific source provider.

The same mutation can now be prepared by the reusable Go source provider:

```sh
make webhook-go-mutation-alpha
```

This strict path binds the generated provider to the exact plan and verifies
the preparation summary before running the campaign.
