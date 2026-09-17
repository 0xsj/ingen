# Experimental TypeScript provider adapter

This package is a cross-language boundary probe for Sorna. It reads exact
`ingen.mutation-plan/v1` bytes and emits an
`ingen.mutation-provider/v1` manifest containing one prepared entry and one
exact capability tuple for each planned mutation.

It does not mutate TypeScript source, build a subject, launch a process, or
implement mutation operators. The command and arguments are supplied by the
caller so the adapter tests only the language-neutral handoff.

Run its focused test directly with:

```sh
bun test sorna/providers/typescript/test/provider.test.mjs
```

Run the cross-language smoke check, including Sorna's Go loader, with:

```sh
make mutation-typescript-provider-conformance
```

The smoke check also runs Sorna's no-execution provider review with mandatory
exact plan binding and writes an `ingen.ci-result/v1` provider-review artifact
under `.artifacts/`. It then sends that envelope through Nublar's aggregate
boundary and checks that the opaque producer report is preserved.
It then collects and reloads the same envelope through Nublar's durable run
store and writes the provider-neutral decision projection.

The adapter is experimental. The strict Go source provider remains Sorna's
current behavioral provider and campaign path.
