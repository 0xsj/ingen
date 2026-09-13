# Notes

**Write only what the code cannot say.**

InGen uses notes to record the language concepts, techniques, and reasoning
that make the implementation worth understanding. The code says what happens;
the note says why this shape exists, what would go wrong without it, and what
principle to carry to the next slice.

## 1. What a note is for

A note answers **why this looks like this** for a reader who can already see
what the code does.

| A note records | A note does not record |
| --- | --- |
| the alternative that was tried and abandoned | a restatement of the function |
| the failure mode a shape prevents | the parameter list |
| the trap invisible at the call site | a summary of the file |
| the principle worth reusing | a defence of known debt |
| what surprised us while building it | a copy of the README or specification |

If the code can already say it clearly, do not write a note about it. If the
reasoning is a product or architecture choice that needs a durable decision,
write a decision record instead. A note may explain a decision's consequence;
it must not become a place to hide an unresolved decision.

## 2. Kinds and lifespans

The kind determines where a note lives and when it becomes stale.

| Kind | Scope of truth | Stale when |
| --- | --- | --- |
| `module` | one package, file, or implementation slice | that code changes |
| `substrate` | a dependency, runtime, OS, or tool at a version | the substrate changes |
| `pattern` | an architectural shape used by InGen | the architecture changes |
| `technique` | a broadly reusable way of working | rarely, but review can overturn it |
| `language` | a language or standard-library mechanism | the language/runtime changes |
| `concept` | an InGen or domain principle | the concept changes |
| `decision-adjacent` | reasoning too large for a decision record | its decision context changes |

Use directories rather than tags:

```text
notes/
  modules/           this code, and usually the largest set
  substrate/         Go, OS, tools, and other things we stand on
  patterns/          architecture that can recur
  techniques/        reusable implementation and testing techniques
  language/          language and standard-library mechanics
  concepts/          domain and verification principles
  decision-adjacent/ reasoning beside a decision record
```

A module note may link to a transferable note. A transferable note must not
depend on a file path or package name from one implementation. If a module note
starts explaining a general principle, extract that principle into the
appropriate transferable directory and link to it.

### Substrate notes have extra obligations

Substrate notes must say:

- which version they describe;
- whether the behavior was measured here or learned from documentation;
- the primary source to revisit when it ages;
- the surprise or failure that made the note worth writing.

An unversioned claim about Go, a database, a browser, or an operating system is
not a durable note. It is a rumor with formatting.

## 3. The claim line

Every note begins with one sentence, before the first heading, that makes a
claim a reader could disagree with.

```markdown
# A passing baseline is not evidence of a useful oracle

A green run establishes compatibility with the current subject, not sensitivity
to a meaningful change in the subject.
```

The title labels the topic. The claim line is what should be remembered. A
claim line that merely repeats the title is not finished.

If a note is unsettled, say so on the claim line:

```text
WORKING — a black-box mutation may be equivalent, but this has not been checked
```

Silence must not turn a hypothesis into settled truth.

## 4. Note anatomy

Use the smallest set of sections that makes the reasoning clear, but do not
omit `Used in`.

```markdown
# <short claim-like title>

<one-sentence claim, before any heading>

## Origin

What caused this note: a bug, failed experiment, implementation surprise,
primary source, or conversation.

## What

The concept, in one paragraph, for someone who has not seen the code.

## Why

The alternative that lost and the failure mode that made it lose.

## Example

The smallest real example, when an example adds understanding.

## Gotchas

What a competent developer is likely to get wrong.

## Used in

The current package, command, experiment, or workflow that relies on this.

## Related

Links to related notes, specifications, or decision records.
```

`Origin` is the memory hook and makes the strength of the claim honest. A
measured observation is different from a documented assumption. `Used in` is
also a maintenance signal: when the named use disappears, the note needs
review.

## 5. Notes versus nearby artifacts

| Artifact | Answers | Changes like |
| --- | --- | --- |
| Code comment | what must be understood at the point of use | the code |
| Specification | what behavior is required | the contract/version |
| README | how to find and use a module | the public surface |
| Decision | what choice was made and what alternatives lost | the decision lineage |
| Note | what principle or surprise the code cannot express | the note's kind |
| Evidence | what happened in a particular run | the run/artifact lineage |

Notes are current understanding. Decisions and evidence preserve historical
lineage. Correcting a note in place is fine, but link to the event or decision
that corrected it when the correction itself is instructive.

## 6. When to write one

Write a note during the work, at the moment something teaches us a principle:

1. A behavior surprises you or a test exposes a non-obvious failure mode.
2. Decide whether the lesson is local, substrate-specific, reusable, or
   domain-level.
3. Write the claim line while the reason is still memorable.
4. Record the losing alternative and the consequence that mattered.
5. Add the smallest example and the exact current use.
6. Link related notes and mark uncertainty explicitly.

There is no requirement to manufacture a note for ordinary code. Roughly one
module note per non-obvious implementation file is a useful default, not a quota.

Writing the note is part of the implementation. If the shape cannot be
explained without hand-waving, that is often evidence that the shape needs
another look.

## 7. InGen-specific emphasis

Notes should make the following ideas learnable as the system grows:

- why the contract is an artifact separate from the implementation;
- what capability isolation does and does not prove;
- why an oracle must be frozen before seeing implementation results;
- how black-box observations differ from source-level assumptions;
- why mutation outcomes need `equivalent`, `invalid`, and `inconclusive` states;
- how evidence hashes establish integrity without proving correctness;
- which Go language and runtime mechanisms make those guarantees possible;
- how Sentinel and CI consume Sorna without reimplementing its semantics.

For Sorna, a note should connect a Go technique to the verification principle
it protects. For Sentinel, it should connect a workflow shape to the trust or
usability problem it solves. The goal is understanding, not documentation
volume.

## 8. Links and review

Use descriptive relative Markdown links for now. Do not cite a note by a bare
number or an ambiguous filename. A future checker may generate a claim index and
verify links, but the note must be useful before that tooling exists.

The [notes README](notes/README.md) is the corpus entry point. It should expose
the categories and useful reading orders, while the notes themselves remain the
source of the claims.
