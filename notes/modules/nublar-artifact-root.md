# Nublar workflow roots and fresh artifacts

Workflow declarations should name result files relative to an artifact root,
not repeat the repository's default `.artifacts/` directory. Nublar resolves
those paths under the root supplied to `aggregate --root` and records the
workflow file separately as provenance.

The repository Makefile uses `.artifacts` as the default root and derives all
generated Sorna, provider, campaign, and Nublar outputs from `ARTIFACT_ROOT`.
The `sorna-ci-result` target includes the clean baseline run, so a fresh root
does not depend on a pre-existing evidence bundle.
When the selected policies cover the alternate location, this permits the
complete workflow to run there without changing the workflow declaration:

```sh
make nublar-aggregate ARTIFACT_ROOT=/private/tmp/ingen-artifacts-run
```

The preferred local entry point is:

```sh
make nublar-aggregate-fresh
```

The persisted run path has a matching clean-workspace entry point:

```sh
make nublar-run-collect-fresh
```

It uses a new artifact root and a new run store, so both producer inputs and
Nublar run history are independent of the caller's existing `.artifacts`.

The fresh targets snapshot the current source tree into a new temporary
workspace, exclude
`.git`, `.artifacts`, and `.cache`, leaves the artifacts available for review,
reuse the configured Go caches for offline-safe dependency resolution, and
print the workspace even when an inner producer fails. Running inside the new
workspace is important: Sorna's filesystem policies are relative to the
workspace and must continue to cover the oracle and subject paths.

The root is an artifact-collection boundary, not a security boundary. Sorna's
subject and oracle policies remain responsible for execution isolation.
