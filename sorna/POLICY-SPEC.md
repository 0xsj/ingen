# Sorna Policy Specification

Status: Draft design specification

This document defines the capability policy artifact that describes what an
Sorna role may read, write, connect to, and invoke. A policy is a declaration
until a host or external runtime enforces it.

## 1. Purpose

The policy makes the intended access boundary reviewable before an enforcement
backend is selected. It must distinguish:

- what access is permitted;
- what access is denied;
- why each permission or denial exists;
- whether the policy is only declared or actually enforced.

The policy is not inferred from an implementation, generated schema, test
output, or agent transcript.

## 2. Lifecycle and enforcement

Policy lifecycle:

```text
draft -> sealed -> superseded
```

The `enforcement` field records the strongest claim supported by the runtime:

- `declared-only`: the policy was reviewed and recorded, but access was not
  host-enforced;
- `host-enforced`: the host applied filesystem, network, process, or tool
  restrictions and recorded their outcomes;
- `externally-attested`: an independent trusted runtime attested to the
  policy and execution.

Sealing canonicalizes the policy and produces a SHA-256 identity. A sealed
policy is immutable; a change creates a new version and hash.

## 3. Machine-readable shape

```yaml
policy:
  schema: ingen.policy/v1
  id: document-pipeline-oracle
  version: 1
  status: draft
  purpose: independent-oracle-generation
  enforcement: declared-only

  filesystem:
    read:
      - path: examples/document-pipeline-lab/contract
        reason: sealed contract and explicitly attached public fixtures
    write:
      - path: .artifacts/document-pipeline-oracle
        reason: frozen oracle output and generation evidence
    deny:
      - path: examples/document-pipeline-lab/subject
        reason: implementation source must not enter oracle generation

  network:
    mode: disabled

  process:
    can_invoke_subject: false
    allowed_tools:
      - name: go
        purpose: deterministic Sorna SDK and oracle tooling
```

Required top-level fields are `schema`, `id`, `version`, `status`, `purpose`,
`enforcement`, `filesystem`, `network`, and `process`.

## 4. Filesystem policy

Each `read`, `write`, and `deny` entry has:

- `path`: a path or root understood by the selected enforcement backend;
- `reason`: the human-readable purpose for the rule.

An exact path may appear only once across all three categories in v1. Parent
and child path overlap is an enforcement-backend concern until path
normalization and symlink policy are defined.

The oracle-generation policy should allow the sealed contract and explicitly
attached public fixtures, isolate oracle output, and deny implementation
worktrees, mutation source, repository metadata, and private transcripts.

## 5. Network policy

`network.mode` is one of:

- `disabled`: no network access is intended;
- `allowlist`: only listed host/port entries are intended;
- `unrestricted`: network access is intentionally not restricted and must be
  treated as a significant assurance limitation.

Allowlist entries require `host`, a non-empty `ports` list between 1 and
65535, and a `purpose`. A DNS name, IP address, or symbolic host is interpreted
by the enforcement backend; policy validation does not resolve it.

## 6. Process and tool policy

`process.can_invoke_subject` states whether the role may invoke the system
under test. Oracle generation must set it to `false`; public observation occurs
only after the oracle is frozen.

`allowed_tools` is optional. Each entry names a tool and explains its purpose.
Tool names are declarations, not proof that the host prevented other tools
from running.

## 7. Evidence requirements

The sealed policy hash should be included in the evidence manifest. The policy
bytes should be copied into the bundle when possible. Evidence should record
whether each relevant access was:

- allowed;
- attempted and denied;
- not attempted;
- unknown to the platform.

`declared-only` must not upgrade a run to capability-isolated assurance.

## 8. First enforcement backend

The first backend is macOS Seatbelt, exposed as:

```sh
sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]
```

It canonicalizes the sandbox root, generates a deny-by-default profile, and
applies the policy's filesystem `read` and `write` roots. The backend also
allows the small set of Darwin runtime paths and bootstrap operations required
by ordinary binaries. Network access is denied by default, so the backend
rejects policies whose network mode is `allowlist` or `unrestricted` until
those modes have a precise implementation.

Explicit filesystem `deny` roots are emitted after the allow rules so a deny
inside a broader allowed root remains denied. Paths are canonicalized before
the profile is generated, including macOS symlinked system roots such as
`/var` and `/private/var`.

This is a capability backend, not yet the complete Sorna oracle lifecycle. It
does not enforce `can_invoke_subject` or `allowed_tools`, does not capture
kernel access events, and does not change the assurance level of `sorna run`.
The host-specific implementation returns an explicit unsupported-backend
error on non-macOS systems.
