# Sorna input-validation mutation checkpoint

## What changed

The document-pipeline Go provider now supports
`input.validation.suffix.add`. The first instance adds `.png` to the supported
suffix condition, making the otherwise unsupported `image.png` case pass
validation while preserving `.txt` support.

The provider matches the `extension != ".txt"` comparison inside the named
validation condition in `createDocument`, not every `.txt` string in the
source. It requires one candidate and records the condition's before/after
meaning in provenance.

## Why it matters

Invalid-input cases are part of the public contract, not optional edge-case
coverage. A subject may pass all valid creation and processing cases while
silently widening its accepted input set. This mutation keeps the valid `.md`
and `.txt` paths unchanged and tests whether the oracle rejects an unsupported
`.png`.

## Result

The fresh campaign killed the mutation through
`document.create.rejects-unsupported-type`. The other rules were unaffected.
The six-mutation source campaign and the prebuilt fixture campaign both
completed without execution errors, and Nublar continued to pass all required
checks.

This is a deliberately small validation mutation. Broader input operators,
such as removing size limits or changing malformed JSON handling, should wait
until the alpha operator vocabulary is frozen and the contract's input
partition is expanded deliberately.
