import { Provenance } from "./provenance.js";
import { setProvenanceAttributes } from "./otel.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const root = Provenance.start();
const recorded: Record<string, string | number> = {};
const span = {
  setAttributes(attributes: Readonly<Record<string, string | number>>): void {
    Object.assign(recorded, attributes);
  },
};

setProvenanceAttributes(span, root);
assert(recorded["amber.execution_id"] === root.execution_id, "execution ID was not set on the span");
assert(recorded["amber.depth"] === 0, "depth was not set on the span");

const retry = root.retry();
setProvenanceAttributes(span, retry);
assert(recorded["amber.mode.kind"] === "retry", "retry mode was not projected");
assert(
  recorded["amber.causation.id"] === root.execution_id,
  "causation was not projected",
);

let rejected = false;
try {
  setProvenanceAttributes(null as unknown as typeof span, root);
} catch {
  rejected = true;
}
assert(rejected, "an invalid span must be rejected");

console.log("TypeScript OpenTelemetry adapter tests passed");
