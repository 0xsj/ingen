# Sorna alpha readiness is an executable stopping point

The Sorna slice had enough capabilities that “done” could otherwise become a
sequence of increasingly broad demos. A readiness check gives the current
vertical a falsifiable boundary instead.

`make sorna-alpha-check` combines four kinds of evidence:

1. all Sorna package tests, including macOS host-enforcement tests when the
   host permits them;
2. Go static analysis for the same packages;
3. syntax validation for the published Sorna JSON schemas; and
4. a fresh-workspace replay matrix that regenerates six intentional defects,
   binds the reviewable expectation manifest, and independently verifies the
   resulting aggregate.

The fourth check matters because package tests can pass while a workflow still
accidentally consumes stale artifacts. The temporary workspace excludes the
repository artifact and cache directories, so the matrix must be produced from
the source inputs it declares.

The first host-enabled readiness run also found a timing bug in the macOS
process sampler: its goroutine could take the first sample before the
synchronous sample promised by `Attach`, causing a short-lived process
transition to be recorded inconsistently. The sampler now starts immediately
for safe shutdown but waits behind the synchronous sample. The test remains
valuable because it checks the ordering guarantee instead of accepting a
timing-dependent result.

This is an alpha stopping point, not a universal correctness claim. Sorna can
show contract sensitivity and artifact integrity here; it cannot prove that a
contract is complete or that an oracle author was externally attested not to
observe an implementation. Those are separate future assurance problems.

The current alpha also deliberately freezes before adding more mutation
operators or language providers. The next additions should be justified by a
concrete missing assurance or interface need, not by expanding the checklist.
