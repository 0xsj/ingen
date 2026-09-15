# Sorna alpha drift guards

## What changed

The alpha verification boundary now has explicit negative tests for campaign
plan drift:

- changing the exact serialized plan bytes is rejected by the exact SHA-256
  reference;
- changing reviewable plan meaning is rejected by the semantic plan identity;
- a provider that declares a different semantic plan identity is blocked during
  no-execution provider review.

## Why both identities matter

The exact hash answers “are these the same bytes?” and protects the run-bound
handoff. The semantic hash answers “does this plan still mean the same thing
across generated baseline run IDs?” It intentionally ignores only the baseline
run ID, not mutation descriptions, targets, expected rules, or input hashes.

That gives the workflow two useful failure modes: ordinary artifact tampering
fails exact binding, while a newly serialized but semantically changed plan
fails semantic review when the old identity is presented.

## Boundary lesson

Negative tests belong next to the consumer that makes the trust decision. A
hash helper test proves hashing behavior; the command-level verifier tests prove
that a campaign result cannot be accepted after its referenced plan changes.
The provider-review test separately proves that a producer cannot claim a
different semantic input set and remain “ready.”

These checks detect drift and inconsistent handoffs. They do not prove that the
original contract is correct, that the provider is independent, or that an
agent never observed implementation details.
