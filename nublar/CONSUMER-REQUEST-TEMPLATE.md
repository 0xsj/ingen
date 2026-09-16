# Nublar consumer request

Copy this template when a concrete consumer needs behavior beyond the frozen
local contract. Keep the request consumer-led: describe the system that will
invoke Nublar or consume its artifacts, not an implementation preference.

## Consumer identity

- Name:
- Owning system or repository:
- Invocation boundary:
- Expected operating environment:

## Current surface

- Existing command or schema used:
- Run identity available to the consumer:
- Producer artifacts already available:
- Current behavior that is insufficient:

## Required behavior

- New input or metadata required:
- Output or projection required:
- Delivery destination and transport:
- Authentication or signing requirement:
- Retry, idempotency, and failure semantics:
- Scale, retention, or concurrency requirement:

## Acceptance proof

- Passing scenario and expected exit code:
- Failed scenario and expected exit code:
- Collection or transport error scenario:
- Persistence and query assertions:
- Evidence that producer-owned meaning remains opaque:

## Scope decision

- Contract or schema files affected:
- Documentation affected:
- Explicit non-goals:
- Regression test or executable consumer proof:

Before implementation, confirm that the requirement cannot be met by the
existing frozen commands and schemas. After implementation, update the
contract checkpoint and run:

```sh
make nublar-freeze-check
```
