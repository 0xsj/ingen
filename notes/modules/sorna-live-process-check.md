# A live process check adds lifecycle evidence without upgrading isolation assurance

Running Sorna against a separately started subject validates the real CLI and HTTP lifecycle, but it does not by itself prove capability isolation.

## Origin

The first live clean and defect runs used separate Go processes on localhost.
The subject was started manually, Sorna connected through the public URL, and
the subject was stopped after each run.

## What

A process-level check exercises compilation, startup, port binding, request
routing, state setup, response capture, artifact writing, and shutdown. It is
stronger evidence of the integration path than an in-process handler test, but
the current runner still receives an already-running URL and records assurance
level 0.

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

- A future Sorna launcher needs readiness, timeout, teardown, and orphan-process
  handling rather than relying on two terminal commands.
- Port availability and process startup are part of the run evidence, not just
  developer convenience.
- The process boundary should not be reported as capability isolation until the
  host enforces and records the relevant permissions.

## Used in

- [`sorna run`](../../sorna/cmd/sorna/)
- [`document-pipeline lab`](../../examples/document-pipeline-lab/)
- the live run artifacts under `.artifacts/`

## Related

- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
- [`The first Sorna runner accepts a subject URL instead of a subject package`](sorna-http-runner.md)
- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
