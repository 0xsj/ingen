# Adversarial boundary probes test negative claims, not just green paths

The trust boundary is only useful if a deliberately bad process is stopped or
classified as unproven. A successful clean run demonstrates behavior; these
probes exercise the conditions that could make the result misleading.

## Origin

Sorna already had a filesystem canary and an unlisted-tool probe. The next
question was whether a binary could be changed after policy preparation while
keeping the same path. That is a direct test of the prepared path/digest versus
the live process identity.

## What changed

The sandbox tests now prepare an executable, replace its bytes before launch,
and require the live identity check to reject the mismatch. The evidence tests
also require the observed executable path and digest to match the prepared
identity before an oracle bundle can be written. A second probe waits until
after the initial identity check, then attempts an unlisted `/bin/echo` exec
from the running process; Seatbelt still denies it.

## Findings

- A path-only check would accept the replacement; the digest check rejects it.
- Process-exec restrictions remain active after startup: a late descendant
  transition to an unlisted executable is denied, not just an immediate probe.
- The identity history records an `exec` transition when a sample catches it,
  but the short-lived-transition probe demonstrates that a process can change
  identity and exit between sampling ticks without appearing in the history.
- The existing filesystem canary proves that an implementation path is denied
  by the host policy, while the unlisted-helper probe proves that process
  execution is not ambient.
- Appending bytes to a signed system binary caused macOS code-signature
  behavior before Sorna could observe it. The probe therefore uses an unsigned
  copy of the test binary, keeping the test about Sorna's identity check rather
  than code signing.

## Limits

- The probes are macOS-specific where they depend on Seatbelt and live process
  inspection; unsupported platforms must report that limitation explicitly.
- A successful launch identity check does not attest later `exec` transitions,
  complete descendant observation, or absence of an access attempt. The late
  probe proves enforcement of one denied transition, while the sampling probe
  makes the allowed-transition observation gap explicit.
- Negative probes should remain separate from contract correctness tests so a
  green behavioral result cannot hide a failed boundary assumption.

## Used in

- [`sorna/internal/sandbox`](../../sorna/internal/sandbox/)
- [`sorna/internal/evidence`](../../sorna/internal/evidence/)
- [`sorna sandbox policy`](../../sorna/POLICY-SPEC.md)
