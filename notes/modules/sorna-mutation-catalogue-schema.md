# Sorna mutation catalogue schema

Date: 2026-09-17

## Decision

Publish `ingen.mutation-catalogue/v1` as a closed JSON Schema for the
reviewable mutation declaration that precedes plan creation. The schema
matches the loader's `mutation_catalogue` wrapper so YAML and JSON producers
share the same structural boundary.

## Boundary

The schema checks required experiment metadata, supported mutation planes,
non-empty operator-specific declarations, reproducible change objects, rule
lists, and lifecycle status. The `change` map remains open because its keys
belong to the named operator.

Sorna's runtime validation remains authoritative for duplicate mutation IDs,
duplicate expected rule IDs, and binding expected rules to the selected
contract. The schema does not claim that a mutation operator is safe,
implemented, or sufficient to measure contract sensitivity.

## Verification

`sorna/spec/catalogue_schema_test.go` loads the repository's document-pipeline
catalogue through the normal Go loader, converts the original YAML wrapper to
JSON, and validates that instance against the published schema. The general
schema test compiles the schema and checks its discriminator and definitions.
