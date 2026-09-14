# A host policy backend needs bootstrap permissions and canonical paths

The policy package gives Sorna a reviewable access declaration. This slice
adds the first host-enforcement backend and exposes it through a small command
so the difference between “declared” and “applied” can be tested directly.

## Origin

The policy artifact could record that the oracle may read the contract but not
the implementation, yet nothing on the host enforced that statement. A real
boundary was needed before Sorna could use capability isolation as evidence.

## What

`internal/sandbox` resolves policy paths relative to a canonical sandbox root
and prepares a command for macOS Seatbelt. The generated profile starts with
deny-by-default, permits the declared read and write roots, and leaves network
access disabled. The CLI entry point is:

```sh
sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]
```

The package test uses `/bin/cat`: it can read a contract file and receives an
OS permission denial when pointed at an implementation file.

Explicit deny roots are emitted after broader allow rules. The test policy
allows its temporary root but denies the `implementation` child, so this
precedence is exercised rather than being an accidental consequence of
deny-by-default.

## Findings

The helper process itself needs more access than the project policy suggests.
On this macOS host, Seatbelt diagnostics showed reads of Darwin's Preboot and
locale paths, `sysctl-read`, and a write plus ioctl on `/dev/dtracehelper`.
These are explicit bootstrap allowances; they do not grant project file
contents or network access.

Path spelling also matters. `/var` is presented as `/private/var` by the
kernel, so the sandbox root is canonicalized before rules are emitted. A
policy can otherwise look correct while failing to match the path the kernel
actually evaluates.

## Why

An enforcement backend has two distinct responsibilities:

1. make the requested project capability boundary real;
2. make the helper executable enough to start and report its result.

Confusing these responsibilities either produces a false “isolated” claim or
an unusable sandbox. The backend currently supports the safe subset of
`network.mode: disabled`; it rejects network allowlists and unrestricted
network policies rather than silently applying the wrong behavior.

## Limits

- Seatbelt support is macOS-specific; other systems return an explicit
  unsupported-backend error.
- `can_invoke_subject` and `allowed_tools` are not yet enforced by this
  command because command identity and tool expansion need their own contract.
- Kernel access events are useful evidence, but are not yet captured in the
  bundle.
- `sorna run` still has assurance level 0 because this backend is not yet
  wired into oracle generation and its lifecycle evidence.

## Used in

- [`sorna/internal/sandbox`](../../sorna/internal/sandbox/)
- [`sorna sandbox exec`](../../sorna/cmd/sorna/)
- [`macOS Seatbelt policy specification`](../../sorna/POLICY-SPEC.md)

## Related

- [`A policy declaration is not an enforcement result`](sorna-policy-definition.md)
- [`A subject URL is not an isolation boundary`](../concepts/a-url-is-not-an-isolation-boundary.md)
