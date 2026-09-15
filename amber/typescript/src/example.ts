import {
  KeyValueStore,
  MapKeyValueBackend,
  ProvenanceContext,
  withOutgoingRequest,
} from "./index.js";
import { createReferenceServiceHandler, ReferenceServiceResponse } from "./reference-service.js";
import { Provenance } from "./provenance.js";

const root = Provenance.start();
const request = withOutgoingRequest(
  new Request("https://example.test/orders/42"),
  root,
);
const store = new KeyValueStore(new MapKeyValueBackend(), "example/amber");

const handler = createReferenceServiceHandler(store);

const response = await handler(request, ProvenanceContext.empty());
if (response.status !== 200) {
  throw new Error(`request returned status ${response.status}`);
}

const body = (await response.clone().json()) as ReferenceServiceResponse;
const stored = await store.get(body.execution_id);
if (!body.stored || stored?.origin !== "incoming") {
  throw new Error("reference service did not persist an incoming child");
}

console.log(`response status: ${response.status}`);
console.log(`response provenance header present: ${response.headers.has("Amber-Provenance")}`);
console.log(`stored child execution: ${body.execution_id}`);
