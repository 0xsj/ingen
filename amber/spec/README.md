# Amber specification

This directory contains the language-neutral Amber model and wire contracts.
Implementations in `go/` and `typescript/` must preserve the behavior defined
here.

## Status

The first draft is version 1 of the core model. It is intentionally small:
framework integrations, persistence formats, and transport-specific adapters
will build on top of it.

## Documents

- [`v1.md`](v1.md) — core concepts, invariants, transitions, and JSON shape
- [`trust-v1.md`](trust-v1.md) — structural validity, trust validation, and the
  unsigned-by-default boundary

## Design rule

An Amber value describes provenance; it does not grant authority, perform
authorization, or claim that its history is complete. Values are immutable once
created. A transition creates a new value and preserves the prior value for
inspection.
