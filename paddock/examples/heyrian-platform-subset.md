# Heyrian platform subset

This policy is a development benchmark for translating Heyrian's existing
`tools/architecture/dependency-cruiser.cjs` rules into Paddock. It deliberately
covers only the rules that can be represented clearly with Paddock's current
component and dependency vocabulary:

- kernel imports only kernel code
- HTTP code imports kernel or HTTP code
- services do not import server, root, framework, cache, feature, content, or UI code
- the root assembles services and kernel code
- only the approved server/root files import server-only code or compose roots
- only the server adapters import Supabase, and only the query tier imports TanStack Query
- portable tiers do not import Node built-ins
- concrete server adapters stay behind the server root, and memory adapters are
  limited to roots, fixtures, and tests
- platform tiers remain acyclic

The source scope mirrors the checked Heyrian TypeScript/Svelte source graph and
excludes lesson example trees. Components intentionally leave unrelated source
unclassified; this keeps the comparison focused on the platform subset.

The translation is not a claim that Paddock has feature parity with
dependency-cruiser. Heyrian's rules include internal target path-family checks
(for example, memory adapter suffixes and adapter folder ownership) and
configuration portability. The current benchmark represents those predicates
with target path and external package-family selectors, while leaving broader
policy-vocabulary differences explicit rather than claiming identical rule
semantics.

To evaluate it from the Heyrian repository:

```sh
go run /path/to/ingen/paddock/cmd/paddock check . \
  --policy /path/to/ingen/paddock/examples/heyrian-platform-subset.yaml \
  --format json
```

The policy should be treated as review material until the translated findings
have been compared with the source dependency-cruiser rules in the target
repository. If dependency-cruiser cannot parse the target's current source
tree, record that run as incomplete evidence rather than treating Paddock's
result as a parity claim.

## Recorded development comparison

The latest complete comparison on 2026-09-16 used the 13-rule Paddock subset.
The source tree is active, so counts are replay measurements rather than fixed
repository facts.

| Tool | In-repository source units | Reported edges/dependencies | Findings |
| --- | ---: | ---: | ---: |
| Paddock | 2,209 | 8,215 | 0 |
| dependency-cruiser | 2,302 total modules | 9,382 total dependencies | 0 |

The 2,209 in-repository TypeScript/Svelte source paths matched exactly. The
edge totals are not expected to match because Paddock keeps external and
unresolved imports as edge target kinds, while dependency-cruiser also
materializes external and asset modules. Both tools reported a clean result;
Paddock evaluated 13 rules and dependency-cruiser evaluated its 13 rules.
