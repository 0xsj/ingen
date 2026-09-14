import { readFile } from "node:fs/promises";
import { Provenance } from "../dist/index.js";

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
