# A managed subject lifecycle improves reproducibility without proving isolation

Sorna can own subject startup, readiness, and teardown while keeping the
subject observable only through its public adapter. That makes the run easier
to reproduce and audit, but it does not yet prevent forbidden filesystem,
network, or process access.

## Origin

The first live runs required two terminal commands and depended on a human to
start and stop the subject. The lifecycle slice moved those actions into
`sorna run`.

## What

Managed mode accepts an executable and explicit arguments, launches it in its
own process group, polls a declared HTTP readiness path until it returns 2xx,
executes the contract, and requests graceful termination after the run. If the
shutdown window expires, Sorna kills the process group and records the
escalation.

## Why

The lifecycle is part of the evidence. A run that cannot show which process
was started, when it became ready, or how it was stopped is harder to replay
and can leave an old subject serving requests to a later run. Explicit command
arguments also avoid making a shell the hidden part of the test boundary.

## Example

```sh
make sorna-run
```

The document subject's `GET /healthz` endpoint is readiness plumbing. It is
separate from the seven contract rules so operational availability does not
silently become product behavior.

## Gotchas

- A successful readiness response proves that an HTTP server is available, not
  that the subject is correct.
- Process-group teardown matters for wrappers such as `go run`, which can
  create a compiled child process.
- Startup and shutdown failures must remain visible in lifecycle evidence;
  they should not be turned into ordinary contract passes.
- Managed process control is not capability isolation. The run remains
  assurance level 0 until the host enforces and attests to access policy.

## Used in

- [`sorna/internal/lifecycle`](../../sorna/internal/lifecycle/)
- [`sorna run`](../../sorna/cmd/sorna/)
- [`document-pipeline lab`](../../examples/document-pipeline-lab/)
- [`lifecycle` field in run artifacts](../../sorna/internal/runner/)

## Related

- [`A live process check adds lifecycle evidence without upgrading isolation assurance`](sorna-live-process-check.md)
- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
- [`Sorna evidence specification`](../../sorna/EVIDENCE-SPEC.md)
