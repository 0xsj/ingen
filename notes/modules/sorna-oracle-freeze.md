# A frozen oracle is a separate artifact from a subject run

The oracle must be created and sealed before subject observations exist, or the
verification process can accidentally turn the implementation into its own
test oracle.

## Origin

The first sandbox backend proved that a process could be denied access to the
implementation, but the only user-facing command was a generic filesystem
probe. This slice needed a real Sorna operation that uses the boundary.

## What

`internal/oracle` turns a sealed contract into canonical `ingen.oracle/v1`
JSON. Each rule becomes a case with a stable case ID, concrete `given` values,
and the contract's `expect` shape. A parent Sorna process starts a separate
child in the sandbox; the child reads the contract and writes `oracle.json`.
The parent then verifies the artifact's contract and policy hashes and writes a
separate oracle evidence bundle.

## Why

Generating the cases in the parent and merely labeling the result “isolated”
would not test the trust boundary. A separate child gives the host a process
to constrain, while the parent remains responsible for orchestration and
artifact verification. The oracle bundle is separate from a subject run so its
lineage cannot be confused with observations produced later by the subject.

## Findings

The child process successfully generated all seven document-pipeline cases
under macOS Seatbelt. The oversized-document generator was expanded to its
concrete 4097-character input, and the resulting artifact carried both the
sealed contract hash and sealed policy hash.

The parent also checks that the contract path is covered by a declared read
root and that the oracle output path is covered by a declared write root. This
binds command arguments to the policy instead of assuming the policy and
command agree.

The oracle bundle now also includes `events/access.jsonl`, populated from the
macOS unified log for the exact sandboxed child PID and execution window. The
manifest records whether that telemetry was captured and whether normalization
had gaps.

## Gotchas

- A frozen oracle is reproducible and traceable; it does not prove that the
  contract is correct.
- The run record now consumes the frozen artifact, but host access events still
  describe the oracle process only; they do not attest to the subject process.
- The unified log is observational. An empty access JSONL is not equivalent to
  no attempted access, and dropped or unavailable telemetry must remain
  visible in assurance status.
- The first generator supports only the `repeat` generator and should reject
  or version future generator kinds explicitly.

## Used in

- [`sorna/internal/oracle`](../../sorna/internal/oracle/)
- [`sorna oracle freeze`](../../sorna/cmd/sorna/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)
- [`document-pipeline oracle specification`](../../sorna/ORACLE-SPEC.md)

## Related

- [`A host policy backend needs bootstrap permissions and canonical paths`](sorna-sandbox-enforcement.md)
- [`A policy declaration is not an enforcement result`](sorna-policy-definition.md)
- [`Fair mutation comparison requires the same sealed contract`](../concepts/fair-mutation-comparison-requires-the-same-contract.md)
