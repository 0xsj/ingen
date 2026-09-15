import { readFile } from "node:fs/promises";
import { Provenance } from "../dist/index.js";
import { createReferenceConsumerHandler } from "../dist/reference-messaging.js";
import { createReferenceServiceHandler } from "../dist/reference-service.js";
import {
  AMBER_PROVENANCE_HEADER,
  PROVENANCE_METADATA_KEY,
  ProvenanceContext,
  decodeHeader,
  decodeMetadata,
  inspectIncomingJSON,
  MemoryStore,
  setProvenanceAttributes,
  StorageConflictError,
  toLogFields,
  toTraceAttributes,
  withOutgoingMessage,
  withOutgoingRequest,
} from "../dist/index.js";

const fixturePath = new URL("../../conformance/v1.json", import.meta.url);
const fixture = JSON.parse(await readFile(fixturePath, "utf8"));

if (fixture.version !== 1) {
  throw new Error(`unexpected fixture version: ${fixture.version}`);
}

for (const testCase of fixture.valid) {
  try {
    Provenance.fromJSON(testCase.value);
  } catch (error) {
    throw new Error(`valid fixture rejected (${testCase.name}): ${String(error)}`);
  }
}

for (const testCase of fixture.invalid) {
  let rejected = false;
  try {
    Provenance.fromJSON(testCase.value);
  } catch {
    rejected = true;
  }
  if (!rejected) {
    throw new Error(`invalid fixture accepted: ${testCase.name}`);
  }
}

const validValues = new Map(fixture.valid.map((testCase) => [testCase.name, testCase.value]));

for (const testCase of fixture.incoming) {
  let input = "";
  if (testCase.input_ref !== undefined) {
    const referenced = validValues.get(testCase.input_ref);
    if (!referenced) {
      throw new Error(`input fixture not found: ${testCase.input_ref}`);
    }
    input = JSON.stringify(referenced);
  } else if (testCase.input_value !== undefined) {
    input = JSON.stringify(testCase.input_value);
  }

  let result;
  let error;
  try {
    result = inspectIncomingJSON(input, testCase.policy);
  } catch (caught) {
    error = caught;
  }
  if (testCase.expect_error) {
    if (!error || result?.present) {
      throw new Error(`expected incoming error: ${testCase.name}`);
    }
    continue;
  }
  if (error) {
    throw new Error(`unexpected incoming error (${testCase.name}): ${String(error)}`);
  }
  if (result.present !== testCase.expected_present) {
    throw new Error(`incoming presence mismatch (${testCase.name})`);
  }
  if (testCase.expected_ref !== undefined) {
    const expected = validValues.get(testCase.expected_ref);
    if (!expected || result.provenance?.execution_id !== expected.execution_id) {
      throw new Error(`incoming value mismatch (${testCase.name})`);
    }
  }
}

function canonical(value) {
  if (Array.isArray(value)) {
    return `[${value.map(canonical).join(",")}]`;
  }
  if (value !== null && typeof value === "object") {
    const keys = Object.keys(value).filter((key) => value[key] !== undefined).sort();
    return `{${keys.map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

for (const testCase of fixture.transitions) {
  const input = validValues.get(testCase.input);
  if (!input) {
    throw new Error(`input fixture not found: ${testCase.input}`);
  }
  const generatedIds = [...testCase.generated_ids];
  const idFactory = () => {
    if (generatedIds.length === 0) {
      throw new Error("deterministic ID sequence exhausted");
    }
    return generatedIds.shift();
  };
  const provenance = Provenance.fromJSON(input, idFactory);
  let result;
  switch (testCase.operation) {
    case "child":
      result = provenance.child();
      break;
    case "retry":
      result = provenance.retry();
      break;
    case "replay":
      result = provenance.replay();
      break;
    default:
      throw new Error(`unsupported operation: ${testCase.operation}`);
  }
  if (generatedIds.length !== 0) {
    throw new Error(`${testCase.name} left generated IDs unused`);
  }
  if (canonical(result.toJSON()) !== canonical(testCase.expected)) {
    throw new Error(
      `transition mismatch (${testCase.name}):\n got ${JSON.stringify(result)}\nwant ${JSON.stringify(testCase.expected)}`,
    );
  }
}

console.log("TypeScript conformance fixtures passed");

const httpFixturePath = new URL("../../conformance/http-v1.json", import.meta.url);
const httpFixture = JSON.parse(await readFile(httpFixturePath, "utf8"));
if (httpFixture.version !== 1 || httpFixture.header_name !== AMBER_PROVENANCE_HEADER) {
  throw new Error("HTTP fixture header contract mismatch");
}
for (const testCase of httpFixture.valid) {
  const result = decodeHeader(testCase.value, "reject");
  if (!result.present || result.provenance === undefined) {
    throw new Error(`valid HTTP fixture rejected: ${testCase.name}`);
  }
}
for (const testCase of httpFixture.invalid) {
  let result;
  let error;
  try {
    result = decodeHeader(testCase.value, testCase.policy);
  } catch (caught) {
    error = caught;
  }
  if (testCase.expect_error) {
    if (!error || result?.present) {
      throw new Error(`expected HTTP fixture error: ${testCase.name}`);
    }
  } else if (error || result.present !== testCase.expected_present) {
    throw new Error(`unexpected HTTP fixture result: ${testCase.name}`);
  }
}
console.log("TypeScript HTTP conformance fixtures passed");

const messagingFixturePath = new URL("../../conformance/messaging-v1.json", import.meta.url);
const messagingFixture = JSON.parse(await readFile(messagingFixturePath, "utf8"));
if (messagingFixture.version !== 1 || messagingFixture.metadata_key !== PROVENANCE_METADATA_KEY) {
  throw new Error("messaging fixture metadata contract mismatch");
}
for (const testCase of messagingFixture.valid) {
  const result = decodeMetadata(testCase.metadata, "reject");
  if (!result.present || result.provenance === undefined) {
    throw new Error(`valid messaging fixture rejected: ${testCase.name}`);
  }
}
for (const testCase of messagingFixture.invalid) {
  let result;
  let error;
  try {
    result = decodeMetadata(testCase.metadata, testCase.policy);
  } catch (caught) {
    error = caught;
  }
  if (testCase.expect_error) {
    if (!error || result?.present) {
      throw new Error(`expected messaging fixture error: ${testCase.name}`);
    }
  } else if (error || result.present !== testCase.expected_present) {
    throw new Error(`unexpected messaging fixture result: ${testCase.name}`);
  }
}
console.log("TypeScript messaging conformance fixtures passed");

const referenceFixturePath = new URL("../../conformance/reference-v1.json", import.meta.url);
const referenceFixture = JSON.parse(await readFile(referenceFixturePath, "utf8"));
if (referenceFixture.version !== 1) {
  throw new Error("reference vertical fixture version mismatch");
}
const referenceRoot = Provenance.start();
const referenceStore = new MemoryStore();
const referenceResponse = await createReferenceServiceHandler(referenceStore)(
  withOutgoingRequest(new Request("https://example.test/orders/42"), referenceRoot),
  ProvenanceContext.empty(),
);
if (referenceResponse.status !== referenceFixture.http.accepted_status) {
  throw new Error(`reference HTTP status mismatch: ${referenceResponse.status}`);
}
const referenceBody = await referenceResponse.json();
const referenceChild = decodeHeader(
  referenceResponse.headers.get(AMBER_PROVENANCE_HEADER),
  "reject",
).provenance;
const referenceStored = await referenceStore.get(referenceBody.execution_id);
if (
  referenceChild === undefined ||
  referenceStored === undefined ||
  referenceChild.execution_id !== referenceBody.execution_id ||
  referenceStored.origin !== referenceFixture.http.child_origin ||
  referenceStored.depth !== referenceRoot.depth + referenceFixture.http.depth_increment ||
  referenceStored.causation?.kind !== referenceFixture.http.causation_kind ||
  (referenceFixture.http.correlation_preserved && referenceStored.correlation_id !== referenceRoot.correlation_id)
) {
  throw new Error("reference HTTP fixture semantics mismatch");
}
const rejectedReferenceResponse = await createReferenceServiceHandler(new MemoryStore())(
  new Request("https://example.test/orders/43", {
    headers: [[AMBER_PROVENANCE_HEADER, "not-base64"]],
  }),
  ProvenanceContext.empty(),
);
if (rejectedReferenceResponse.status !== referenceFixture.http.rejected_status) {
  throw new Error("reference HTTP rejection status mismatch");
}

const consumerRoot = Provenance.start();
const consumerStore = new MemoryStore();
const consumerResponse = await createReferenceConsumerHandler(consumerStore)(
  withOutgoingMessage({ body: "order" }, consumerRoot),
  ProvenanceContext.empty(),
);
const consumerChild = decodeMetadata(consumerResponse.metadata, "reject").provenance;
const consumerStored = consumerChild === undefined
  ? undefined
  : await consumerStore.get(consumerChild.execution_id);
if (
  consumerResponse.body !== `order${referenceFixture.messaging.processed_body_suffix}` ||
  consumerChild === undefined ||
  consumerStored === undefined ||
  consumerStored.origin !== referenceFixture.messaging.child_origin ||
  consumerStored.depth !== consumerRoot.depth + referenceFixture.messaging.depth_increment ||
  consumerStored.causation?.kind !== referenceFixture.messaging.causation_kind ||
  (referenceFixture.messaging.correlation_preserved && consumerStored.correlation_id !== consumerRoot.correlation_id)
) {
  throw new Error("reference messaging fixture semantics mismatch");
}
console.log("TypeScript reference vertical conformance fixture passed");

const loggingFixturePath = new URL("../../conformance/logging-v1.json", import.meta.url);
const loggingFixture = JSON.parse(await readFile(loggingFixturePath, "utf8"));
if (loggingFixture.version !== 1) {
  throw new Error("logging fixture version mismatch");
}
const loggingProvenance = Provenance.fromJSON(loggingFixture.value);
if (canonical(toLogFields(loggingProvenance)) !== canonical(loggingFixture.expected)) {
  throw new Error(
    `logging field mismatch:\n got ${JSON.stringify(toLogFields(loggingProvenance))}\nwant ${JSON.stringify(loggingFixture.expected)}`,
  );
}
console.log("TypeScript logging conformance fixture passed");

const tracingFixturePath = new URL("../../conformance/tracing-v1.json", import.meta.url);
const tracingFixture = JSON.parse(await readFile(tracingFixturePath, "utf8"));
if (tracingFixture.version !== 1) {
  throw new Error("tracing fixture version mismatch");
}
const tracingProvenance = Provenance.fromJSON(tracingFixture.value);
if (canonical(toTraceAttributes(tracingProvenance)) !== canonical(tracingFixture.expected)) {
  throw new Error(
    `tracing attribute mismatch:\n got ${JSON.stringify(toTraceAttributes(tracingProvenance))}\nwant ${JSON.stringify(tracingFixture.expected)}`,
  );
}
console.log("TypeScript tracing conformance fixture passed");

const otelFixturePath = new URL("../../conformance/otel-v1.json", import.meta.url);
const otelFixture = JSON.parse(await readFile(otelFixturePath, "utf8"));
if (otelFixture.version !== 1) {
  throw new Error("OpenTelemetry fixture version mismatch");
}
const otelProvenance = Provenance.fromJSON(otelFixture.value);
const otelAttributes = {};
setProvenanceAttributes(
  { setAttributes: (attributes) => Object.assign(otelAttributes, attributes) },
  otelProvenance,
);
if (canonical(otelAttributes) !== canonical(otelFixture.expected)) {
  throw new Error(
    `OpenTelemetry attribute mismatch:\n got ${JSON.stringify(otelAttributes)}\nwant ${JSON.stringify(otelFixture.expected)}`,
  );
}
console.log("TypeScript OpenTelemetry conformance fixture passed");

const storageFixturePath = new URL("../../conformance/storage-v1.json", import.meta.url);
const storageFixture = JSON.parse(await readFile(storageFixturePath, "utf8"));
if (storageFixture.version !== 1) {
  throw new Error("storage fixture version mismatch");
}
const storage = new MemoryStore();
const storageValue = Provenance.fromJSON(storageFixture.value);
await storage.put(storageValue);
const storedValue = await storage.get(storageValue.execution_id);
if (storedValue?.execution_id !== storageValue.execution_id) {
  throw new Error("storage fixture value could not be read");
}
const conflictValue = Provenance.fromJSON(storageFixture.conflict_value);
let storageConflict = false;
try {
  await storage.put(conflictValue);
} catch (error) {
  storageConflict = error instanceof StorageConflictError;
}
if (!storageConflict) {
  throw new Error("storage conflict fixture was not rejected");
}
if (await storage.get(storageFixture.missing_execution_id) !== undefined) {
  throw new Error("missing storage fixture value was found");
}
const historyStore = new MemoryStore();
for (const value of storageFixture.history.values) {
  await historyStore.put(Provenance.fromJSON(value));
}
const history = await historyStore.listByWorkId(storageFixture.history.work_id);
if (history.length !== storageFixture.history.expected_execution_ids.length) {
  throw new Error("storage history length mismatch");
}
for (let index = 0; index < history.length; index += 1) {
  if (history[index].execution_id !== storageFixture.history.expected_execution_ids[index]) {
    throw new Error(`storage history order mismatch at ${index}`);
  }
}
const causal = await historyStore.listByCausation(
  storageFixture.causation.kind,
  storageFixture.causation.id,
);
if (causal.length !== storageFixture.causation.expected_execution_ids.length) {
  throw new Error("storage causation history length mismatch");
}
for (let index = 0; index < causal.length; index += 1) {
  if (causal[index].execution_id !== storageFixture.causation.expected_execution_ids[index]) {
    throw new Error(`storage causation order mismatch at ${index}`);
  }
}
const correlated = await historyStore.listByCorrelationId(storageFixture.correlation.id);
if (correlated.length !== storageFixture.correlation.expected_execution_ids.length) {
  throw new Error("storage correlation history length mismatch");
}
for (let index = 0; index < correlated.length; index += 1) {
  if (correlated[index].execution_id !== storageFixture.correlation.expected_execution_ids[index]) {
    throw new Error(`storage correlation order mismatch at ${index}`);
  }
}
console.log("TypeScript storage conformance fixture passed");
