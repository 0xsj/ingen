import {
  AMBER_PROVENANCE_HEADER,
  decodeHeader,
  withOutgoingRequest,
} from "./http.js";
import { ProvenanceContext } from "./context.js";
import { Provenance } from "./provenance.js";
import { MemoryStore } from "./storage.js";
import {
  createReferenceServiceHandler,
  ReferenceServiceResponse,
} from "./reference-service.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const store = new MemoryStore();
const root = Provenance.start();
const request = withOutgoingRequest(
  new Request("https://example.test/orders/42"),
  root,
);
const response = await createReferenceServiceHandler(store)(
  request,
  ProvenanceContext.empty(),
);
assert(response.status === 200, `accepted request returned ${response.status}`);
const body = (await response.json()) as ReferenceServiceResponse;
assert(body.stored && body.execution_id !== "", "service did not confirm storage");
const childHeader = response.headers.get(AMBER_PROVENANCE_HEADER);
assert(childHeader !== null, "service response omitted child provenance");
const decodedChild = decodeHeader(childHeader, "reject").provenance;
assert(decodedChild?.execution_id === body.execution_id, "response header did not carry the child");
const stored = await store.get(body.execution_id);
assert(stored?.origin === "incoming", "stored service child has the wrong origin");
assert(stored.causation?.id === root.execution_id, "stored service child has the wrong causation");

const topLevelResponse = await createReferenceServiceHandler(new MemoryStore())(
  new Request("https://example.test/orders/43"),
  ProvenanceContext.empty(),
);
assert(topLevelResponse.status === 200, "top-level request should create local provenance");

const rejected = await createReferenceServiceHandler(new MemoryStore())(
  new Request("https://example.test/orders/44", {
    headers: [[AMBER_PROVENANCE_HEADER, "not-base64"]],
  }),
  ProvenanceContext.empty(),
);
assert(rejected.status === 400, `malformed request returned ${rejected.status}`);
assert(
  rejected.headers.get(AMBER_PROVENANCE_HEADER) === null,
  "rejected request returned an Amber provenance header",
);
assert((await rejected.text()) === "invalid Amber provenance\n", "malformed response body changed");

const trusted = Provenance.start();
const untrusted = Provenance.start();
const trustedStore = new MemoryStore();
const validator = (provenance: Provenance): void => {
  if (provenance.execution_id !== trusted.execution_id) {
    throw new Error("execution is not trusted");
  }
};
const trustedHandler = createReferenceServiceHandler(trustedStore, validator);
const trustedResponse = await trustedHandler(
  withOutgoingRequest(new Request("https://example.test/orders/trusted"), trusted),
  ProvenanceContext.empty(),
);
assert(trustedResponse.status === 200, "trusted request should reach the service");
const untrustedResponse = await trustedHandler(
  withOutgoingRequest(new Request("https://example.test/orders/untrusted"), untrusted),
  ProvenanceContext.empty(),
);
assert(untrustedResponse.status === 400, "untrusted request should be rejected");
assert(
  (await trustedStore.listByWorkId(untrusted.work_id)).length === 0,
  "untrusted request should not be stored",
);

console.log("TypeScript reference service tests passed");
