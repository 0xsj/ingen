# A subject URL is not an isolation boundary

Observing a subject through HTTP separates the runner from its implementation process, but it does not prove that the oracle or runner lacked forbidden access.

## Origin

The first runner records assurance level 0 because it accepts an already-running
subject URL and does not enforce or attest to filesystem, network, workspace, or
process capabilities.

## What

Transport separation answers where observations are collected: through the
public interface. Isolation answers what the observing actors were allowed to
read, write, execute, and communicate with while producing those observations.
The latter requires host-enforced policy and evidence about enforcement; it
cannot be inferred from the URL alone.

## Why

Calling an HTTP endpoint is a valuable black-box testing technique, but a
runner on the same unrestricted host could still inspect the subject source,
read its database, or influence its environment before making the request.
Claiming independence from the transport shape alone would overstate the run's
assurance.

## Example

The current run record says `adapter: http-json-v1` and
`assurance.level: 0`. It records the limitation that the subject was supplied
as an already-running URL instead of claiming capability isolation.

## Gotchas

- A process boundary can reduce accidental coupling without proving information
  flow was impossible.
- Self-reported access logs are evidence about what was reported, not a complete
  proof of what the host permitted.
- Isolation evidence must distinguish an action that was not attempted from one
  that was attempted and denied.

## Used in

- [`sorna/internal/runner`](../../sorna/internal/runner/)
- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
- [`isolation threat model`](../../ISOLATION-THREAT-MODEL.md)

## Related

- [`The first Sorna runner accepts a subject URL instead of a subject package`](../modules/sorna-http-runner.md)
- [`The contract can cross language boundaries`](the-contract-can-cross-language-boundaries.md)
