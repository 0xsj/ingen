# Process execution is a capability, not an ambient convenience

The filesystem and network parts of a policy already describe where a process
may reach. Process execution needs the same treatment: an isolated child must
not be able to turn an otherwise narrow policy into a shell or an unreviewed
tool runner.

## Origin

The first identity slice gave policies a logical `subject_id`, but that alone
could still be paired with an arbitrary executable at launch. The next useful
claim was to preserve the concrete binary path and digest that Sorna prepared.

## What changed

`internal/sandbox` resolves `process.allowed_tools` with `exec.LookPath`,
canonicalizes the resulting executable paths, and passes them to the macOS
Seatbelt profile. The profile allows `process-exec` for the requested command
and those tools only. An unlisted helper process receives an OS denial, which
is tested with a real Seatbelt launch.

The prepared boundary and the evidence manifest record both the semantic
`subject_id`, `can_invoke_subject` value, resolved executable path and digest,
and the resolved tool paths. Sorna checks that `subject_id` matches the sealed
contract ID before an oracle or subject run proceeds. The policy hash still
binds the original reviewable names and purposes; the prepared record shows
what the host backend actually resolved and the evidence checksum binds that
record to the run.

## Why

Allowing a command to start is different from allowing every executable it can
find. Making the command path explicit gives the host backend a deny-by-default
process capability. It also creates a useful failure mode: a missing or
unresolvable declared tool stops preparation rather than silently widening or
narrowing the runtime boundary.

The initial command is necessarily allowed to start. Therefore
`can_invoke_subject: false` cannot, by itself, mean “the entry process may not
be the subject”; it means the role must not invoke a separately identified
subject. The new `subject_id` gives that subject a stable logical identity and
prevents policy/contract mix-ups. The recorded executable digest associates the
identity with the launch artifact, but the boolean remains a semantic
constraint until the host independently attests the executable or process
namespace.

## Descendant evidence

Access collection attaches to the root PID and samples Darwin's process table
while it runs. It walks parent-child relationships and keeps the observed PID
set, then filters unified-log Seatbelt events to that set. This is more useful
than attributing every event to the wrapper PID, especially when a declared
tool is a short-lived child.

The sampler is deliberately described as observation: it can miss a child that
starts and exits between samples, and it does not create a process namespace or
prove that the process table was complete. Sampling failures are counted in
`process_tree_errors` and downgrade oracle telemetry to an observed-with-gaps
status. The evidence therefore remains assurance level 0.

## Limits

- The current process-exec implementation is macOS Seatbelt-specific.
- Declaring a tool does not grant its libraries, configuration, or data; those
  still need filesystem capabilities.
- The launch path and digest are recorded evidence, not an independent OS
  attestation of the running process.
- `can_invoke_subject` still needs a host-level process namespace or equivalent
  attestation before it can be enforced as a distinct capability.
- Process-tree sampling improves attribution but is not complete observation.

## Used in

- [`sorna/internal/sandbox`](../../sorna/internal/sandbox/)
- [`oracle evidence`](../../sorna/internal/evidence/)
- [`managed subject lifecycle`](sorna-subject-isolation.md)
