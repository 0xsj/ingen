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

Both policies now carry `process.subject_id`, and Sorna checks that value
against the sealed contract ID before launching the relevant process. This is
a logical identity binding; it is not yet an attested mapping from the ID to
every OS process in the subject's tree. The sandbox also records the resolved
launch executable and digest, which binds the evidence to a concrete artifact
without independently attesting the running process.

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
- Seatbelt now restricts a managed subject to its prepared executable and any
  explicitly resolved `allowed_tools`. The executable is still allowed as the
  initial process even when `can_invoke_subject` is false; that field describes
  whether the role may invoke a named subject, which is not yet a distinct
  process identity in this policy.
- Access events include the root PID and any descendants observed by the
  sampler. This improves attribution for helper processes without claiming
  complete process-tree observation.
- The run remains assurance level 0. Host enforcement and host logs are useful
  evidence, but they do not independently attest to complete observation or
  subject identity enforcement.

## Limits

- The current backend is macOS Seatbelt-specific and supports TCP host/port
  rules, not unrestricted network access.
- `can_invoke_subject` remains semantic until the logical subject identity can
  be independently bound to an OS executable or process namespace.
- Descendant correlation is best-effort sampling and can miss short-lived
  processes; it is not an attested process namespace.
- An already-running external URL cannot receive this subject policy.

## Used in

- [`sorna run`](../../sorna/cmd/sorna/)
- [`subject policy`](../../examples/document-pipeline-lab/policy/subject.yaml)
- [`lifecycle`](../../sorna/internal/lifecycle/)
- [`evidence`](../../sorna/internal/evidence/)
