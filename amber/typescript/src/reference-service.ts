import {
  HTTPHandler,
  httpMiddlewareWithValidator,
  withOutgoingResponse,
} from "./http.js";
import type { IncomingValidator } from "./incoming.js";
import { Provenance } from "./provenance.js";
import type { ProvenanceStore } from "./storage.js";

export type ReferenceServiceResponse = {
  readonly execution_id: string;
  readonly stored: boolean;
};

/**
 * Build the Fetch-compatible application boundary used by the TypeScript
 * reference example. Storage and the server/runtime remain application-owned.
 */
export function createReferenceServiceHandler(
  store: ProvenanceStore,
  validator?: IncomingValidator,
): HTTPHandler {
  if (store === undefined || typeof store.put !== "function") {
    throw new TypeError("reference service store must implement put");
  }

  return httpMiddlewareWithValidator(async (_request, context) => {
    const parent = context.provenance ?? Provenance.start();
    const child = parent.child({ origin: "incoming" });
    await store.put(child);
    const body: ReferenceServiceResponse = {
      execution_id: child.execution_id,
      stored: true,
    };
    const response = new Response(JSON.stringify(body), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
    return withOutgoingResponse(response, child);
  }, "reject", validator);
}
