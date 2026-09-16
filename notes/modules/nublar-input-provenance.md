# A coordinator should bind result paths to consumed bytes

Knowing that a workflow expected `sorna-ci-result.json` is not enough. A file
at that path can be replaced between collection and review, or two runs can
reuse the same path with different contents.

Nublar now records a SHA-256 for every CI-result file immediately after loading
and validating it. The aggregate keeps both the configured path and the full
producer artifact. The hash is an integrity reference for the bytes Nublar
consumed; it is not a new correctness or isolation claim.

This is deliberately separate from the producer's internal input hashes. The
Sorna envelope may reference its evidence bundle, while Nublar references the
Sorna envelope file it actually aggregated. Each layer proves a different
link in the artifact chain.

## Limits

The current aggregate report does not store a signed attestation or an
external content-addressed artifact store. The separate local run store uses
content-addressed filenames for immutable records, while these local SHA-256
references preserve the handoff boundary for a future retention or remote-CI
layer.
