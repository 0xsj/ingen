# Ecosystem HTTP acceptance fixtures

Run from the repository root on macOS with Go and Rust installed:

```sh
make ecosystem-http-check
```

The driver creates a fresh `/private/tmp/ingen-ecosystem-http.*` project. Set
`INGEN_ECOSYSTEM_HTTP_ROOT` to an absolute, nonexistent path to select the
destination. It needs permission for Sorna's macOS sandbox and loopback HTTP
listeners. It preserves failed-run evidence and stops only its own webhook
receiver.

The driver builds its contract from Malcolm's checked-in `document_flow.malc`
and copies the repository's document HTTP subject into an isolated temporary
project. `approval-record.test-only.template.json` is a synthetic, checked-in
governance fixture. The driver substitutes only the digest of the generated
sealed contract; it does not create or claim an operator approval.

`paddock.policy.yaml` is a test policy for the same copied HTTP subject source.
It checks the `cmd/document-pipeline` composition edge and forbids imports from
the service package into composition code. It is a test policy and is not a
sealed production policy. The fresh project's command entry point uses
`document-pipeline-main.go`, a small acceptance-only listener wrapper that
handles Sorna's stop signal with `http.Server.Shutdown`; the checked-in service
implementation and response behavior remain the subject under test. Sorna's
Seatbelt profile requires a `localhost` network rule, while the server binds
the corresponding literal `127.0.0.1` address to avoid hostname resolution in
the sandbox.

The workflow preserves exact before, defect, and webhook-delivery bytes in
Lockwood and requires an acyclic custody verification before the final
comparison. The expected generated campaign contains one real mutation, which
changes the accepted document response from HTTP 202 to HTTP 200 and must be
killed by the contract oracle.

`SUMMARY.txt` and `acceptance-index.json` identify the final evidence root,
exact contract/oracle and CLI hashes, producer outcomes, graph/mutation/custody
counts, and assurance limits. The project retains producer logs and envelopes,
raw Amber contexts and execution captures, complete prepared mutation source
and binary snapshots, Lockwood custody, immutable Nublar runs/decisions and
delivery receipts, and Sattler comparison inputs/results. Required custody IDs
are explicit; exact duplicate artifacts do not create cyclic lineage.

The driver requires a passing clean decision, a failed defective decision,
accepted local delivery of that failed decision, and expected rejections for
missing governance, a forbidden architecture edge, a damaged custody parent,
and a missing required producer result. Sattler's behavioral and custody
comparisons must be compatible; the distinct Amber work IDs remain an explicit
provenance incompatibility in its overall result.

Amber records coordinator lineage through Sentinel's public SDK integration.
Coordinator execution is `not-isolated` and `unverified`; the HTTP subject does
not consume that context. This workflow does not establish fresh native agent
context, independent attestation, Linux containment, or remote delivery. Native
Herdr acceptance has a separate evidence gate.
