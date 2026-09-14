import { ProvenanceContext } from "./context.js";
import {
  PROVENANCE_METADATA_KEY,
  MessageMetadata,
  decodeMetadata,
  decodeMetadataWithValidator,
  messageMiddleware,
  messageMiddlewareWithValidator,
  withIncomingMetadata,
  withIncomingMessage,
  withOutgoingMetadata,
  withOutgoingMessage,
} from "./messaging.js";
import { Provenance } from "./provenance.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start();
const source: MessageMetadata = { "trace-id": "trace-1" };
const outgoing = withOutgoingMetadata(source, root);
assert(source[PROVENANCE_METADATA_KEY] === undefined, "source metadata was mutated");
assert(outgoing[PROVENANCE_METADATA_KEY] !== undefined, "outgoing metadata is missing provenance");

const decoded = decodeMetadata(outgoing, "reject");
assert(decoded.present && decoded.provenance?.execution_id === root.execution_id, "metadata round trip failed");

const empty = ProvenanceContext.empty();
const installed = withIncomingMetadata(empty, outgoing, "reject");
assert(installed.present && installed.context.provenance?.execution_id === root.execution_id, "incoming metadata was not installed");
const absent = withIncomingMetadata(installed.context, undefined, "reject");
assert(!absent.present && absent.context === installed.context, "absent metadata must preserve context");

assert(!decodeMetadata({ [PROVENANCE_METADATA_KEY]: "bad!" }, "ignore").present, "invalid ignored metadata must be absent");
let rejected = false;
try {
  decodeMetadata({ [PROVENANCE_METADATA_KEY]: "bad!" }, "reject");
} catch {
  rejected = true;
}
assert(rejected, "invalid rejected metadata must throw");

const incomingMessage = withIncomingMessage(
  { body: "input", metadata: outgoing },
  empty,
  "reject",
);
assert(
  incomingMessage.present && incomingMessage.context.provenance?.execution_id === root.execution_id,
  "incoming message context was not installed",
);
assert(incomingMessage.message.body === "input", "incoming message body changed");

const outgoingMessage = withOutgoingMessage(
  { body: "output", metadata: { reply: "yes" } },
  root,
);
assert(outgoingMessage.metadata?.[PROVENANCE_METADATA_KEY] !== undefined, "outgoing message is missing provenance");

let called = false;
const wrapped = messageMiddleware(
  (message, context) => {
    called = true;
    assert(message.metadata !== outgoing, "incoming metadata should be cloned");
    assert(context.provenance?.execution_id === root.execution_id, "message middleware context is incorrect");
    return { body: `${message.body}-handled`, metadata: { reply: "yes" } };
  },
  "reject",
);
const handled = await wrapped({ body: "input", metadata: outgoing }, empty);
assert(called && handled.body === "input-handled", "message middleware did not call the consumer");
assert(handled.metadata?.[PROVENANCE_METADATA_KEY] !== undefined, "message middleware did not propagate metadata");

let rejectedCalled = false;
try {
  await messageMiddleware(
    () => {
      rejectedCalled = true;
      return { body: "unexpected" };
    },
    "reject",
  )({ body: "bad", metadata: { [PROVENANCE_METADATA_KEY]: "bad!" } }, empty);
} catch {
  // Rejected message metadata must stop the consumer chain.
}
assert(!rejectedCalled, "rejected message metadata reached the consumer");

let ignoredCalled = false;
const ignoredMessage = await messageMiddleware(
  (message, context) => {
    ignoredCalled = true;
    assert(context.provenance === undefined, "ignored metadata should not install provenance");
    return message;
  },
  "ignore",
)({ body: "ignored", metadata: { [PROVENANCE_METADATA_KEY]: "bad!" } }, empty);
assert(ignoredCalled && ignoredMessage.body === "ignored", "ignored metadata should continue the consumer chain");
assert(ignoredMessage.metadata?.[PROVENANCE_METADATA_KEY] === "bad!", "ignored metadata should remain unchanged");

const validator = (provenance: Provenance): void => {
  if (provenance.execution_id !== root.execution_id) {
    throw new Error("execution is not trusted");
  }
};
const verifiedMetadata = decodeMetadataWithValidator(outgoing, "reject", validator);
assert(verifiedMetadata.present, "trusted metadata was rejected");
const untrusted = Provenance.start();
const untrustedMetadata = withOutgoingMetadata(undefined, untrusted);
assert(!decodeMetadataWithValidator(untrustedMetadata, "ignore", validator).present, "ignored untrusted metadata must be absent");

let untrustedCalled = false;
try {
  await messageMiddlewareWithValidator(
    () => {
      untrustedCalled = true;
      return { body: "unexpected" };
    },
    "reject",
    validator,
  )({ body: "bad", metadata: untrustedMetadata }, empty);
} catch {
  // Rejected trust validation must stop the consumer chain.
}
assert(!untrustedCalled, "untrusted metadata reached the consumer");

console.log("TypeScript messaging adapter tests passed");
