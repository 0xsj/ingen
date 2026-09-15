import { ProvenanceContext } from "./context.js";
import {
  decodeMetadata,
  withOutgoingMessage,
} from "./messaging.js";
import { Provenance } from "./provenance.js";
import { MemoryStore } from "./storage.js";
import { createReferenceConsumerHandler } from "./reference-messaging.js";

const store = new MemoryStore();
const root = Provenance.start();
const handler = createReferenceConsumerHandler(store, (value) => {
  if (value.execution_id !== root.execution_id) {
    throw new Error("execution is not trusted");
  }
});
const output = await handler(
  withOutgoingMessage({ body: "order", metadata: { queue: "orders" } }, root),
  ProvenanceContext.empty(),
);
if (output.body !== "order-processed" || output.metadata === null || output.metadata === undefined) {
  throw new Error("reference consumer did not process the message");
}

const child = decodeMetadata(output.metadata, "reject").provenance;
if (child === undefined) {
  throw new Error("reference consumer response omitted provenance");
}
const stored = await store.get(child.execution_id);
if (stored?.origin !== "incoming" || stored.causation?.id !== root.execution_id) {
  throw new Error("reference consumer did not persist an incoming child");
}

console.log(`consumer body: ${output.body}`);
console.log(`consumer response provenance metadata present: ${output.metadata["Amber-Provenance"] !== undefined}`);
console.log(`stored consumer execution: ${stored.execution_id}`);
