# Webhook mutation fixtures

These binaries are deliberately small, prebuilt defect variants. They are
used to prove the Sorna campaign boundary before a webhook-specific source
provider exists.

The `accepts-duplicate` variant changes only the second delivery of an event:
it returns the accepted response again instead of the contracted duplicate
response. The clean subject remains the implementation under test; the wrapper
is the controlled mutation.
