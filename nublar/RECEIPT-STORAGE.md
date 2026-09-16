# Nublar delivery receipt storage

The optional local receipt store preserves delivery outcomes independently from
immutable run records:

```sh
nublar run deliver \
  --store .artifacts/nublar-runs \
  --run-id <run-id> \
  --webhook https://example.test/nublar \
  --receipt-store .artifacts/nublar-receipts

nublar run receipt list \
  --receipt-store .artifacts/nublar-receipts \
  --run-id <run-id> --status failed --transport http-webhook
```

The store records accepted deliveries and failed attempts when the publisher
was reached. Configuration and projection failures occur before a publisher
attempt and therefore do not create a receipt.

## Filesystem behavior

- Each filename is `receipt-<sha256>.json`, where the digest covers the exact
  validated JSON bytes.
- Publication uses a temporary file, sync, close, and same-filesystem link.
- A second save of identical receipt bytes is rejected; existing receipts are
  never overwritten.
- Listing validates the closed receipt shape and verifies every filename hash.
- `run receipt list` supports exact-match `--run-id`, `--status` (`accepted`
  or `failed`), and `--transport` filters. They are independently optional and
  composable.
- `run receipt list` returns `0` for an empty store and for a list containing
  failed delivery receipts. It returns `2` for storage, parsing, validation,
  or output errors.

Receipts are audit evidence for delivery attempts, not Nublar run decisions.
They do not change the stored run, and this boundary does not select retry
policy, authentication, retention, or remote storage.
