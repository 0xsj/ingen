# A managed subject policy is separate from an oracle policy

The process that generates the oracle and the process that serves the public
subject have different capabilities. They should not share one policy merely
because Sorna starts both of them.

## What changed

`sorna run` accepts `--subject-policy` for a managed subject. The policy is
sealed independently and copied to `policy/subject/` in the subject run
evidence. Its `host-enforced` declaration is recorded alongside the runtime
backend result; the result is not inferred from the fact that Sorna launched a
process. The lifecycle record keeps the original subject command, while the
host-wrapped command remains an implementation detail of the enforcement
backend.

The document-pipeline example now builds a subject binary before starting it.
The runtime policy can read that binary directory, deny the contract, defect,
oracle, and Git roots, and allow only the declared local HTTP listener. The
subject's host access events are written to `events/subject-access.jsonl`,
separate from the oracle bundle's `events/access.jsonl`.

## Why

Oracle generation needs contract inputs and no subject access. A subject needs
its executable and a public listener, but it should not gain the oracle's
contract workspace simply because both processes are part of one run. Separate
policy hashes make this distinction reviewable and prevent evidence consumers
from treating one process's observations as proof about the other.

## Findings

- A compiled subject is a clearer runtime boundary than `go run`: the Go tool
  and source tree do not need to be present in the subject's sandbox.
- A listening HTTP subject needs both bind and inbound capabilities; an
  outbound-only host/port allowlist is insufficient.
- The policy schema now carries an optional network direction. Omitting it
  preserves the original outbound allowlist behavior.
- The run remains assurance level 0. Host enforcement and host logs are useful
  evidence, but they do not independently attest to complete observation or
  process/tool isolation.

## Limits

- The current backend is macOS Seatbelt-specific and supports TCP host/port
  rules, not unrestricted network access.
- Process and tool declarations are still policy data rather than enforced
  controls.
- Access telemetry is scoped to the launched process ID; descendant process
  correlation is a later slice.
- An already-running external URL cannot receive this subject policy.

## Used in

- [`sorna run`](../../sorna/cmd/sorna/)
- [`subject policy`](../../examples/document-pipeline-lab/policy/subject.yaml)
- [`lifecycle`](../../sorna/internal/lifecycle/)
- [`evidence`](../../sorna/internal/evidence/)
