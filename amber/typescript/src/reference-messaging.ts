import {
  Message,
  MessageHandler,
  messageMiddlewareWithValidator,
  withOutgoingMessage,
} from "./messaging.js";
import type { IncomingValidator } from "./incoming.js";
import { Provenance } from "./provenance.js";
import type { ProvenanceStore } from "./storage.js";

/** Build the Fetch/runtime-neutral consumer used by the messaging example. */
export function createReferenceConsumerHandler(
  store: ProvenanceStore,
  validator?: IncomingValidator,
): MessageHandler<string> {
  if (store === undefined || typeof store.put !== "function") {
    throw new TypeError("reference consumer store must implement put");
  }

  return messageMiddlewareWithValidator(async (message, context) => {
    const parent = context.provenance ?? Provenance.start();
    const child = parent.child({ origin: "incoming" });
    await store.put(child);
    const output: Message<string> = {
      body: `${message.body}-processed`,
      metadata: { reply: "yes" },
    };
    return withOutgoingMessage(output, child);
  }, "reject", validator);
}
