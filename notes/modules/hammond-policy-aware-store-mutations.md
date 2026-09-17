# Store mutations must use the record's referenced policy

A record's policy reference is part of its governance identity. Registration
already loads and validates the referenced policy, but later event appends,
amendments, and supersessions must use that same policy when deriving the next
state.

## What

The file store reloads `record.Policy` before applying any lifecycle mutation
and calls `AppendEventWithPolicy`. This preserves custom authority mappings and
quorum rules across the full CLI workflow. Falling back to
`DefaultReviewPolicy()` is only valid for records explicitly using that policy.

## Why

Without this rule, a custom-policy record could register successfully and then
fail or change meaning on its next mutation because the store silently applied
the default policy. The failure is especially easy to hit with the solo local
authority workflow, where the actor is intentionally not the default example
reviewer.

## Used in

- `hammond/internal/store/filesystem.go`
- `hammond/cmd/hammond/main_test.go`
- `hammond/examples/solo/`
