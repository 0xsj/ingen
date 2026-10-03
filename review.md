# InGen — external review

Date: 2026-09-17
Reviewed at: commit `5f2cfb8` ("feat: v1.0 alpha release"), branch `dev`
Reviewer: external agent review, no prior context on this repo
Audience: an agent picking up remediation work, or a maintainer triaging it

---

## 0. How to use this document

Sections 1–3 are context and verdict. **Section 4 is the work**: numbered tasks,
each with a repro command, the exact files involved, and an acceptance check an
agent can run to confirm it is done. Section 5 is the strategic call, which is a
human decision and should not be actioned by an agent without the owner's sign-off.

Every claim in this document was verified by execution at the commit above. Where
a finding is a judgment call rather than a fact, it is labeled `[judgment]`.

---

## 1. What was verified

| Check | Command | Result |
| --- | --- | --- |
| Root Go build | `go build ./...` | clean |
| Root Go vet | `go vet ./...` | clean |
| Root Go tests | `go test ./...` | all pass |
| Amber Go tests | `cd amber/go && go test ./...` | all pass |
| Malcolm Rust tests | `cd malcolm && cargo test` | 51 pass |
| Flagship workflow | `make nublar-aggregate-fresh` | **FAILS** — see task 1 |
| Core coverage | `go test -cover ./core/... ./sorna/internal/{contract,oracle,mutation}` | 65–67% |

Scale at time of review:

- 483 Go files / 100,278 lines (2 modules: `ingen`, `github.com/0xsj/ingen/amber`)
- 23 Rust files / 4,183 lines (`malcolm`)
- 87 TypeScript files / 3,170 lines (`amber/typescript`)
- 349 Markdown files / 34,357 lines
- 43 commits spanning 2026-09-13 to 2026-09-17

---

## 2. Verdict

**The thesis is correct and underserved. The engineering discipline in the core is
better than most commercial code. The project's problem is not quality — it is
that the valuable part is being buried under ~6x its volume in unproven surface,
with no CI to protect any of it.**

To answer the question directly: **yes, this is a genuinely complex attempt at a
real problem — and the complexity is currently the main risk to it.** The problem
(tests and implementation produced from the same context form a closed circle, so
green tests are weak evidence) is real, correctly stated, and not well served by
existing tools. The complexity that is *essential* to solving it — contract
sealing, oracle isolation, mutation challenge, evidence integrity — is roughly
26K lines in `sorna/` plus `core/` and `malcolm/`. The complexity that is
*incidental* — six further named "verticals" with no users — is roughly 57K lines.
The ratio is inverted relative to where the value sits. `[judgment]`

---

## 3. What is genuinely strong — preserve these

These are assets. Remediation work must not damage them.

1. **The thesis.** "Green tests are weak evidence unless the contract is
   independently meaningful" is a correct, non-obvious, underserved observation.
   Mutation-killing is the right falsifiable proof of it.

2. **`ISOLATION-THREAT-MODEL.md`** is the best artifact in the repository. Its §10
   distinction between *observed* non-access, *enforced* non-access, *unobserved
   access possibility*, and *attested* non-access is real security thinking, and
   the rule that only the strongest **supported** statement may appear in a report
   is exactly right. The three assurance tiers
   (`independence-unverified` / `capability-isolated` / attested) should be treated
   as load-bearing, not aspirational prose.

3. **The dependency graph is clean.** Verified by import scan: every vertical
   imports only `core`, plus a single `sorna` edge in `herdr-sentinel`. No cycles.
   At 100K lines that is rare and it is what keeps this codebase salvageable.

4. **The demo proves something falsifiable.** The document-pipeline lab kills six
   semantic mutations (202→200, dropped required `name`, 400→500, stuck `queued`,
   wrong persistence key, PNG accepted). That is evidence, not a claim.

5. **The refusal to over-claim is consistent** across `status.md` and the threat
   model: mutation killing proves *sensitivity, not correctness*; hashes prove
   *integrity, not that an agent never looked*. Protect this. It is the project's
   credibility and it is the thing most easily lost in a marketing pass.

---

## 4. Findings and remediation tasks

Ordered by priority. Tasks 1–3 are blocking for any claim of "alpha release."

---

### Task 1 — `[BLOCKING]` The flagship demo is broken at HEAD

**Severity:** critical. This is the first command an evaluator runs.

`README.md` advertises `make nublar-aggregate-fresh` as "the complete clean
workflow." It fails after ~7 seconds:

```
invalid mutation catalogue binding:
- mutation_catalogue.contract_version 2 does not match contract.version 3
make[1]: *** [mutation-catalogue-validate] Error 1
```

**Cause:** `examples/document-pipeline-lab/contract/contract.yaml:4` declares
`version: 3`. It was bumped in commit `5f2cfb8` — the commit named
"feat: v1.0 alpha release". `examples/document-pipeline-lab/mutations/catalogue.yaml:6`
still declares `contract_version: 2`; it was last touched two commits earlier in
`35801ff`.

**Repro:**
```sh
make nublar-aggregate-fresh
```

**Fix:** reconcile the catalogue against contract v3. Do not blindly bump the
integer — contract v3 introduced `additional_properties: false` on response
shapes (per `status.md`), so confirm the existing six mutations still bind to
valid rule IDs under v3 before changing the version field.

**Acceptance:**
```sh
make nublar-aggregate-fresh   # must exit 0
```
All four required Nublar checks must pass, and all six document-pipeline
mutations must still be reported as killed. A run that passes because mutations
were removed is a regression, not a fix.

---

### Task 2 — `[BLOCKING]` There is no CI protecting the mainline

**Severity:** critical. This is the root cause of Task 1 and of every future
Task 1.

The repository has three workflows. None of them guards `dev` or `main`:

| File | Trigger | Covers |
| --- | --- | --- |
| `.github/workflows/nublar-consumer.yml` | `workflow_dispatch` only | manual, never automatic |
| `.github/workflows/paddock-release.yml` | `push: tags: paddock-v*` | release only |
| `.github/workflows/sorna-release.yml` | tags, `workflow_dispatch`, path-filtered PRs | sorna/core/nublar/examples on PR only |

Consequences, all verified:

- **Nothing runs `go test ./...` on push.** The only PR trigger is path-filtered
  and the project history shows direct commits to `dev` with no PRs, so it has
  never fired.
- **Seven of nine modules have zero automated coverage**: `hammond`, `lockwood`,
  `paddock`, `sattler`, `amber`, `malcolm`, `herdr-sentinel`.
- **The one producer job cannot fail.** `.github/workflows/nublar-consumer.yml`
  lines 25–40 combine `continue-on-error: true`, `set +e`, and `make -k`. It is
  structurally incapable of reporting failure.

`[judgment]` A project whose entire premise is rigorous verification has the
weakest CI configuration of any codebase I have reviewed at this scale. That is
worth fixing for its own sake and for what it signals.

**Fix:** add `.github/workflows/ci.yml` triggered on `push` and `pull_request`
for all branches, running at minimum:

```sh
go build ./...
go vet ./...
go test ./...
(cd amber/go && go test ./...)
(cd malcolm && cargo test)
make sorna-alpha-check
```

Note the platform constraint from Task 3: `make sorna-alpha-check` and anything
touching `sorna/internal/sandbox` currently require `macos-latest`. Split the job
— run the platform-independent build/vet/test matrix on `ubuntu-latest`, and the
sandbox-dependent targets on `macos-latest` — so that most of the suite runs at
Linux runner cost.

Separately, remove `continue-on-error: true` from the `producer` job in
`nublar-consumer.yml`, or document explicitly why a producer failure must not
fail that workflow.

**Acceptance:** a commit that reintroduces the Task 1 contract-version mismatch
must cause a red build.

---

### Task 3 — The core security control is macOS-only

**Severity:** high. Strategic, not cosmetic.

Oracle isolation — the enforcement that makes the entire product claim credible —
has exactly one backend:

- `sorna/internal/sandbox/sandbox_darwin.go:15` — shells out to
  `/usr/bin/sandbox-exec` with a generated seatbelt profile
- `sorna/internal/sandbox/access_darwin.go:136` — collects telemetry via
  `/usr/bin/log show`

On every other platform, the build-tagged fallbacks return errors:

```go
// sorna/internal/sandbox/sandbox_unsupported.go   (//go:build !darwin)
return Prepared{}, fmt.Errorf("no host enforcement backend is available on this operating system")

// sorna/internal/sandbox/access_unsupported.go    (//go:build !darwin)
return nil, fmt.Errorf("no host access telemetry backend is available on this operating system")
```

Three consequences worth facing directly:

1. **The product cannot enforce its value proposition where CI actually lives.**
   Both sandbox-dependent workflow jobs pin `runs-on: macos-latest`, at roughly
   10x the per-minute cost of Linux runners on GitHub Actions.
2. **`sandbox-exec` has been marked deprecated in its own man page for years.**
   The entire enforcement story rests on an API Apple has signalled it will remove.
3. **Without it, every run degrades to `independence-unverified`** — which the
   project's own threat model (§8) correctly identifies as the weakest tier and
   the one that must not be presented as attestation.

**Fix `[judgment]`:** prototype a Linux enforcement backend behind the existing
`preparePlatform` / `startAccessCapture` seams. The abstraction in
`sorna/internal/sandbox/access.go` is already correctly shaped for this — it is a
platform interface, not a macOS interface, which is good design that has not yet
been used. Candidate mechanisms, roughly in order of effort:

- a container boundary with an explicit read-only mount set and no network
- `bubblewrap` (`bwrap`) for filesystem and network namespacing
- Landlock (Linux 5.13+) for unprivileged filesystem restriction
- seccomp-bpf for syscall narrowing, if telemetry needs it

**Acceptance:** `sorna oracle freeze` completes on `ubuntu-latest` with the run
labeled `capability-isolated` rather than `independence-unverified`, and a
negative test proves that an oracle process attempting to read a denied
implementation root is refused and the denial is recorded.

This is the single highest-leverage engineering item available. It converts a
macOS demo into a deployable product.

---

### Task 4 — Scope has outrun proof by roughly 6x `[judgment]`

Nine named "verticals," of which eight are directories inside one Go module
(`module ingen`). Line counts:

| Module | Go (incl. tests) | Markdown | Note |
| --- | --- | --- | --- |
| `sorna` | 26,044 | 3,140 | the differentiating engine |
| `lockwood` | 18,880 | 2,775 | no user |
| `paddock` | 17,038 | 3,968 | no user |
| `sattler` | 8,721 | 756 | no user |
| `hammond` | 7,720 | 1,345 | no user |
| `herdr-sentinel` | 7,385 | 763 | coordination |
| `amber` | 6,239 | 6,674 | more docs than code |
| `nublar` | 5,798 | 2,747 | self-described placeholder |
| `malcolm` (Rust) | 4,109 | 608 | spec front end |
| `core` | 443 | 278 | interchange layer |

`status.md` already concedes the pattern for at least one of these:

> "Nublar currently acts as a thin coordinator and proof surface. It is
> intentionally not the final Nublar product; a later rewrite can make it a
> standalone product owned and designed separately."

That is a placeholder for a product with no user, surrounded by 25 design
documents about its boundaries.

`MODULES.md` already contains the correct governing rule and the project is not
currently following it:

> "Split a surface only when it has a distinct trust boundary, deployment model,
> ownership model, or user workflow."

**Recommendation (requires owner sign-off — see §5).** Do not action unilaterally.

---

### Task 5 — Documentation has become a liability

**Severity:** medium, compounding.

349 Markdown files totalling 34,357 lines — one third the volume of the Go.
Concrete instances:

- **`nublar/`** ships 25 `.md` files for 5,798 lines of Go, including *both*
  `USAGE.md` and `USAGE-GUIDE.md`, *both* `STORAGE.md` and `STORAGE-EVALUATION.md`,
  *both* `EXECUTION-BOUNDARY.md` and `EXECUTION-EVALUATION.md`, plus five separate
  `*-BOUNDARY.md` documents (`DELIVERY-`, `EXECUTION-`, `OUTPUT-`, `PRODUCT-`,
  and the `CONTRACT-CHECKPOINT`).
- **`amber/`** has 93 `.md` files at 6,674 lines against 6,239 lines of Go — more
  prose than program.
- **`status.md`** is a 100+ bullet append-only list. It is a changelog wearing a
  status document's name. It cannot answer "where is this project?" for a new
  contributor or for the author in three months.
- `notes/` holds a further 154 files.

`[judgment]` The signature here is that each work session produced a *new*
document rather than editing an existing one. That is cheap to generate and
expensive to own. The cross-references between the duplicate pairs are real (e.g.
`USAGE-GUIDE.md` does point back at `USAGE.md`), so this is sprawl rather than
pure duplication — but the effect on a newcomer is the same.

**Fix:**
1. Rewrite `status.md` as one page: what works / what is proven / what is next /
   what is known broken. Move the bullet log verbatim to `CHANGELOG.md`.
2. Collapse each module to one `README.md` plus one `spec/` directory. Merge the
   duplicate pairs listed above. Fold the five `*-BOUNDARY.md` files into one
   `BOUNDARIES.md` per module.
3. Delete evaluation documents whose decision has already been made and recorded
   elsewhere (e.g. `STORAGE-EVALUATION.md` concludes "evaluated and deferred" —
   that is one line in a roadmap, not a 60-line file).

**Acceptance:** total tracked `.md` count under 80, with no information loss for
anything still true. Verify with `git ls-files '*.md' | wc -l`.

---

### Task 6 — InGen does not eat its own dog food

**Severity:** medium. Highest marketing leverage of any item here.

Every Sorna subject is an example lab — `document-pipeline-lab`,
`webhook-validation-lab`, and the Malcolm-lowered variants. Verified by scanning
every `--subject-root` and `--subject-command` in the 907-line `Makefile`: none
targets InGen's own packages.

This means InGen's own 100,278 lines of Go, and the ~39K lines of tests written
alongside them by the same context, are **precisely the closed circle the project
exists to break**. Coverage on the core packages is 65–67%, which is respectable
but is measuring the wrong thing by the project's own argument.

**Fix:** pick one Sorna package with a settled `doc.go` contract — `core/ciresult`
is a good first candidate at 443 lines with an existing published schema
(`core/ciresult-v1.schema.json`) — and run the full cycle against it: freeze an
oracle from the specification alone, run the black-box subject, and mutation-test
the result.

The repository already has the tooling for exactly this: the `spec-tests` skill
and the `spec-test-writer` agent enforce the information barrier this requires.

**Acceptance:** a `make` target that runs Sorna against an InGen package and
reports a mutation kill ratio. Publish that ratio in `README.md`. If mutations
survive, that is a *better* result than a clean pass — it is the tool finding real
gaps in its own tests, which is the most persuasive possible demonstration.

---

## 5. The strategic call — owner decision, do not action unilaterally

`[judgment]` **Decide what InGen is.**

The reviewer's read: InGen is **Sorna** — contract-frozen, isolation-enforced,
mutation-challenged verification — with `core/` as its interchange layer and
`malcolm/` as its specification front end. That is ~31K lines, it works, it
demonstrably kills real mutations, and nothing else on the market does it.

`hammond`, `lockwood`, `nublar`, `sattler`, and `amber` are ~47K lines of
speculative surface. Each is individually plausible. None has a user. Each is now
something that must be maintained, documented, and kept compiling.

**Recommended:** move them to `future/` or a separate repository and stop
maintaining them until a concrete consumer asks. `MODULES.md` already grants the
permission to do this; `nublar/STORAGE-EVALUATION.md` already models the right
instinct ("evaluated and deferred").

**The counter-case, stated fairly:** the modules are genuinely well-decoupled
(verified — clean import graph, everything depends only on `core`), they all
compile and pass tests, and the marginal cost of leaving them in place is lower
than it would be in a tangled codebase. If the intent is to demonstrate a complete
ecosystem vision rather than to ship one tool, the current shape is defensible.
That is a product decision, not an engineering one.

---

## 6. Context an agent should carry into this work

43 commits between 2026-09-13 and 2026-09-17 — approximately 20,000 lines of Go
per day. That velocity explains every pattern in this review: breadth chosen over
depth, documents accumulated as session artifacts rather than edited, and no CI
because nothing has yet had time to break twice.

This is context, not criticism. The remediation is narrowing and consolidating,
not building more. An agent picking up this document should resist the instinct to
add surface. **Tasks 1, 2, 3, 5, and 6 are all subtraction or hardening. None of
them requires a new module.**

---

## 7. Task summary

| # | Task | Severity | Blocking alpha? | Agent-actionable? |
| --- | --- | --- | --- | --- |
| 1 | Fix broken `nublar-aggregate-fresh` contract version | critical | yes | yes |
| 2 | Add CI on push/PR; un-silence nublar producer job | critical | yes | yes |
| 3 | Linux sandbox enforcement backend | high | yes | yes, scoped |
| 4 | Scope reduction | high | no | **no — owner sign-off** |
| 5 | Documentation consolidation | medium | no | yes |
| 6 | Run Sorna against InGen | medium | no | yes |

Suggested order: 1 → 2 → 5 → 6 → 3, with 4 decided by the owner in parallel.
Task 1 is a few minutes. Task 2 is an hour and prevents the next Task 1.
