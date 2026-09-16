# The Malcolm contract must be sealed before Sorna can produce behavioral evidence

The first Malcolm-to-Sorna run is meaningful only when the generated contract, frozen oracle, oracle policy, subject policy, and observed subject are kept as separate bound artifacts.

## Origin

The IR adapter proved structural compatibility, but a validated draft contract
is not yet a behavioral experiment. Sorna's existing workflow already has the
needed phases: seal the contract, freeze the oracle, build and manage a subject,
then execute the frozen oracle through the public HTTP boundary.

## What

The malcolm-sorna-run Make target compiles the healthcheck source, translates
its IR into a draft Sorna contract, seals that contract, freezes an oracle under
the dedicated oracle policy, builds the clean document subject, and runs the
oracle against GET /healthz under a separate subject policy. The oracle policy
can read the translated contract and write oracle evidence but cannot invoke the
subject. The subject policy can read the compiled subject binary but denies the
Malcolm contract and oracle artifacts. The target then verifies the completed
Sorna run bundle's recorded hashes.

## Why

Using the draft contract directly at run time would make it possible for the
contract source or translation output to change between review and execution.
Using the source again after freezing would also blur the oracle boundary. The
run therefore consumes the frozen oracle artifact, while Sorna records the
sealed contract and policy identities in the resulting evidence bundle.

The healthcheck is intentionally narrow. It proves the handoff and lifecycle
boundaries for a stateless case. Malcolm now has separate request-body,
stateful-setup, target-negative, and event-assertion lowering proofs; negative
setup assertions still need a Sorna-compatible lowering.

## Example

~~~text
Malcolm source
  -> malcolm.ir/v1
  -> draft Sorna contract
  -> sealed contract
  -> frozen oracle
  -> managed document subject /healthz
  -> Sorna run bundle
~~~

Run it with:

~~~sh
make malcolm-sorna-run
~~~

## Gotchas

- Contract validation is only a structural check. A passing behavioral run is
  evidence about the declared endpoint, not universal correctness.
- The generated contract's identity is document_health, so both policies use
  that same subject ID. Sorna rejects mismatched policy/contract identity.
- The oracle and subject policies are deliberately different. Sharing one
  policy would make their capability boundaries harder to review.
- macOS host enforcement may require the local Seatbelt permission needed by
  Sorna's sandbox backend; an unavailable backend must not be reported as a
  host-enforced pass.
- The output directories are artifacts of the run and should be fresh when
  evidence immutability or reproducibility is being evaluated.

## Used in

- [malcolm/examples/healthz/oracle-policy.yaml](../../../malcolm/examples/healthz/oracle-policy.yaml)
- [malcolm/examples/healthz/subject-policy.yaml](../../../malcolm/examples/healthz/subject-policy.yaml)
- Makefile target malcolm-sorna-run
- [malcolm/examples/healthz.malcolm](../../../malcolm/examples/healthz.malcolm)
- [sorna/cmd/sorna](../../../sorna/cmd/sorna/main.go)

## Related

- [Malcolm reaches Sorna through a rejecting IR adapter](malcolm-sorna-bridge.md)
- [A frozen oracle is a separate artifact from a subject run](sorna-oracle-freeze.md)
- [A subject run consumes the frozen oracle, not the contract source](sorna-run-consumes-frozen-oracle.md)
- [Notes protocol](../../../NOTES.md)
