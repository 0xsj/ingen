import { ProvenanceContext } from "./context.js";
import {
  PROVENANCE_METADATA_KEY,
  decodeMetadata,
  withOutgoingMessage,
} from "./messaging.js";
import { Provenance } from "./provenance.js";
import { MemoryStore } from "./storage.js";
import { createReferenceConsumerHandler } from "./reference-messaging.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const store = new MemoryStore();
const root = Provenance.start();
const output = await createReferenceConsumerHandler(store)(
  withOutgoingMessage({ body: "order", metadata: { queue: "orders" } }, root),
  ProvenanceContext.empty(),
);
assert(output.body === "order-processed", "consumer changed the wrong body");
const encodedChild = output.metadata?.[PROVENANCE_METADATA_KEY];
assert(encodedChild !== undefined, "consumer response omitted provenance metadata");
const child = decodeMetadata(output.metadata, "reject").provenance;
assert(child?.origin === "incoming", "consumer response has the wrong origin");
assert(child?.causation?.id === root.execution_id, "consumer response has the wrong causation");
assert((await store.get(child.execution_id)) !== undefined, "consumer did not persist the child");

const topLevel = await createReferenceConsumerHandler(new MemoryStore())(
  { body: "order" },
  ProvenanceContext.empty(),
);
assert(topLevel.body === "order-processed", "top-level message did not create local provenance");

const trusted = Provenance.start();
const untrusted = Provenance.start();
const trustedStore = new MemoryStore();
const validator = (value: Provenance): void => {
  if (value.execution_id !== trusted.execution_id) {
    throw new Error("execution is not trusted");
  }
};
const trustedHandler = createReferenceConsumerHandler(trustedStore, validator);
await trustedHandler(
  withOutgoingMessage({ body: "trusted" }, trusted),
  ProvenanceContext.empty(),
);
let rejected = false;
try {
  await trustedHandler(
    withOutgoingMessage({ body: "untrusted" }, untrusted),
    ProvenanceContext.empty(),
  );
} catch {
  rejected = true;
}
assert(rejected, "untrusted message should be rejected");
assert(
  (await trustedStore.listByWorkId(untrusted.work_id)).length === 0,
  "untrusted message should not be stored",
);

console.log("TypeScript reference consumer tests passed");
