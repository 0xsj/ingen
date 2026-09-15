# Sorna persistence mutation checkpoint

## What changed

The document-pipeline provider now supports
`state.persistence.key.replace`. The mutation changes the accepted-document
assignment from `h.store.docs[id]` to `h.store.docs["mutation-discarded"]`, while
leaving the response's document ID unchanged.

The provider matches the receiver, map field, key identifier, and stored
`document` literal. It requires exactly one candidate before replacing only the
index expression and records the source location and before/after meaning.

## Why it matters

An accepted response is not enough evidence that persistence is correct. The
returned identifier must be usable by the next public operation. This mutation
keeps the create response green but makes the follow-up `GET /documents/{id}`
fail, which tests the contract's cross-request consistency.

## Result

The fresh campaign killed the mutation through
`document.status.exposes-queued-state`. The process rule also failed because
its setup received the unreadable identifier, while the result rule became
inconclusive because completion could not be established. The create and
invalid-input rules remained unaffected.

This is a useful reminder that one defect can create multiple direct-looking
failures across a stateful oracle. The declared expected rule remains the
classification anchor; the additional observations explain the blast radius.
