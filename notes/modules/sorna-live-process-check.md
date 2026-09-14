# A live process check adds lifecycle evidence without upgrading isolation assurance

Running Sorna against a separately started subject validates the real CLI and HTTP lifecycle, but it does not by itself prove capability isolation.

## Origin

The first live clean and defect runs used separate Go processes on localhost.
The subject was initially started manually, Sorna connected through the public
URL, and the subject was stopped after each run. The next slice moved that
lifecycle into Sorna itself.

## What

A process-level check exercises compilation, startup, port binding, readiness,
request routing, state setup, response capture, artifact writing, and shutdown.
Sorna now owns that lifecycle when `--subject-command` is supplied; an explicit
external-URL mode remains available for comparison. Both modes record
assurance level 0; a managed subject can now have host enforcement, but that
does not replace independent attestation.

The launch command is resolved to an absolute executable path and hashed before
start. On macOS, Sorna compares that identity with a host observation of the
live root process or its descendants after startup. Oracle generation passes a
startup-gate file descriptor to its child, so the child cannot read the
contract until this check succeeds. A mismatch aborts the run before the
behavioral result is accepted.

## Why

An injected transport can prove evaluator logic without proving that the
compiled subject starts or that requests cross a real process boundary. At the
same time, a localhost URL cannot prove that the runner was prevented from
reading the subject workspace or influencing its environment.

## Example

```text
clean process  -> contract pass, 7/7 rules pass
defect process -> contract fail, status-200-create killed
```

## Gotchas

- Managed runs need a declared readiness endpoint, startup timeout, shutdown
  timeout, and process-group teardown rather than relying on two terminal
  commands.
- Port availability and process startup are part of the run evidence, not just
  developer convenience.
- The process boundary should not be reported as capability isolation until the
  host enforces and independently attests the relevant permissions.
- The executable check and history are still parent-side observations. The
  history can record an observed `exec` transition, but does not prove that a
  process cannot later change identity or that every short-lived descendant
  was observed.

## Used in

- [`sorna run`](../../sorna/cmd/sorna/)
- [`document-pipeline lab`](../../examples/document-pipeline-lab/)
- the live run artifacts under `.artifacts/`

## Related

- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
- [`The first Sorna runner accepts a subject URL instead of a subject package`](sorna-http-runner.md)
- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
