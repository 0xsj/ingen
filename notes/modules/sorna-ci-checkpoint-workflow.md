# Sorna CI checkpoint workflow

Date: 2026-09-17

## Decision

The repository CI boundary invokes the existing `make sorna-release-check`
target rather than recreating Sorna assertions inside YAML. The workflow uses
a macOS runner because the current sandbox evidence includes Darwin process
observation, installs Bun for the experimental TypeScript provider adapter,
and uploads the fresh provider-to-Nublar artifacts when the job finishes.

## Fresh workspace rule

`SORNA_RELEASE_WORKSPACE` is an optional Makefile input. When omitted,
`sorna-release-check` creates a unique directory under `/private/tmp`. When
provided, the directory must be empty; a non-empty directory is rejected
instead of allowing stale output files to satisfy the checkpoint. CI supplies
`${{ runner.temp }}/ingen-sorna-release-check`, which makes the resulting
artifacts available to the upload step while preserving the same freshness
invariant.

## Scope

This workflow proves the current Sorna release checkpoint in CI. It does not
claim independent attestation of oracle generation, universal language
support, or a native Herdr binding. Those remain separate product and trust
boundaries.
