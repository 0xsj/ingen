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
    subject_id: document-pipeline
    can_invoke_subject: false
    allowed_tools:
      - name: go
        purpose: deterministic Sorna SDK and oracle tooling
```

Required top-level fields are `schema`, `id`, `version`, `status`, `purpose`,
`enforcement`, `filesystem`, `network`, and `process`. The process object
requires `subject_id` and `can_invoke_subject`.

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
by the enforcement backend; policy validation does not resolve it. An entry may
also declare `direction` as `inbound`, `outbound`, or `both`. If omitted, v1
treats it as `outbound` for compatibility with the original policy shape.
Direction is part of the capability: a managed subject that listens on a local
port must declare inbound access explicitly.

## 6. Process and tool policy

`process.subject_id` is the stable logical identity of the system under test
that this role refers to. Sorna binds it to the sealed contract ID before
execution, so a policy cannot silently be reused for another contract. The
prepared execution also records the resolved executable path and SHA-256 so
the logical identity is associated with a concrete launch artifact.

`process.can_invoke_subject` states whether the role may invoke the system
under test. Oracle generation must set it to `false`; public observation occurs
only after the oracle is frozen. The logical binding does not by itself attest
that every OS process belongs to that subject; the host backend still needs a
process identity or independent executable attestation for that stronger
claim. A recorded path and digest establish bundle lineage, not what an OS
process may have executed after launch.

`allowed_tools` is optional. Each entry names a tool and explains its purpose.
The macOS Seatbelt backend resolves each name before launch and restricts
`process-exec` to the requested command plus those resolved tool paths. This
does not make the tool's own dependencies available automatically; they must
also be covered by the filesystem capabilities. Other backends may expose a
different enforcement result.

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
by ordinary binaries. Network access is denied by default. The current
allowlist implementation applies TCP host/port entries with explicit
inbound, outbound, or both direction; unrestricted network access remains
unsupported.

Explicit filesystem `deny` roots are emitted after the allow rules so a deny
inside a broader allowed root remains denied. Paths are canonicalized before
the profile is generated, including macOS symlinked system roots such as
`/var` and `/private/var`.

This is a capability backend, not yet the complete Sorna oracle lifecycle. It
enforces command/tool execution paths on macOS. Sorna also binds the policy's
logical `subject_id` to the sealed contract ID, while
`can_invoke_subject` remains a semantic permission until a host-level subject
process identity can be enforced. Oracle freezes query macOS unified-log
Seatbelt events into `events/access.jsonl`, while managed subject runs keep
their observations in `events/subject-access.jsonl`. The collector samples the
process tree so events can be attributed to the root and observed descendants,
but sampling is best-effort rather than an attestation. Both streams are host
observations rather than a proof of absence. The host-specific implementation
returns an explicit unsupported-backend error on non-macOS systems.
