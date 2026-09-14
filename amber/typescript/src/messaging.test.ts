import { ProvenanceContext } from "./context.js";
import {
  PROVENANCE_METADATA_KEY,
  MessageMetadata,
  decodeMetadata,
  withIncomingMetadata,
  withOutgoingMetadata,
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

console.log("TypeScript messaging adapter tests passed");
