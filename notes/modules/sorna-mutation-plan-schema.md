# Sorna mutation plan schema

Date: 2026-09-17

## Decision

Publish `ingen.mutation-plan/v1` as a closed JSON Schema. The plan is the
provider handoff that binds the exact catalogue, contract, oracle, baseline,
policy, and ordered implementation mutations, so downstream SDKs need a
language-neutral structural contract before they can safely consume it.

## Boundary

The schema checks field types, required references, digest shape, ready status,
implementation-plane mutations, and mutation declaration structure. Sorna's
Go runtime remains authoritative for cross-field identity checks, contiguous
sequence numbers, catalogue semantics, and canonical JSON bytes.

The `change` object remains open because mutation operators own their
language- and target-specific parameters. Opening that one value map does not
open the surrounding plan or mutation envelope.

## Verification

`sorna/spec/plan_schema_test.go` constructs a valid plan through Sorna's own
types, serializes it canonically, and validates the resulting instance against
the published schema. The general schema test also compiles the schema and
checks its required definitions and discriminator.
