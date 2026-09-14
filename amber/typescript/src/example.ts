import {
  KeyValueStore,
  MapKeyValueBackend,
  Provenance,
  ProvenanceContext,
  httpMiddleware,
  mergeLogFields,
  toTraceAttributes,
  withOutgoingRequest,
} from "./index.js";

const root = Provenance.start();
const request = withOutgoingRequest(
  new Request("https://example.test/orders/42"),
  root,
);
const store = new KeyValueStore(new MapKeyValueBackend(), "example/amber");

const handler = httpMiddleware(async (_request, context) => {
  const incoming = context.provenance;
  if (incoming === undefined) {
    return new Response("missing provenance", { status: 500 });
  }

  const child = incoming.child({ origin: "incoming" });
  await store.put(child);
  const body = {
    child_execution_id: child.execution_id,
    log_fields: mergeLogFields({ component: "example" }, child),
    trace_attributes: toTraceAttributes(child),
  };
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}, "reject");

const response = await handler(request, ProvenanceContext.empty());
if (response.status !== 200) {
  throw new Error(`request returned status ${response.status}`);
}

console.log(`response status: ${response.status}`);
console.log(`response provenance header present: ${response.headers.has("Amber-Provenance")}`);
console.log(`stored child response: ${await response.text()}`);
