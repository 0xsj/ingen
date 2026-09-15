import {
  AlwaysOnSampler,
  BasicTracerProvider,
  InMemorySpanExporter,
  SimpleSpanProcessor,
} from "@opentelemetry/sdk-trace-base";
import { Provenance } from "./provenance.js";
import { setProvenanceAttributes } from "./otel.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}

const exporter = new InMemorySpanExporter();
const provider = new BasicTracerProvider({
  sampler: new AlwaysOnSampler(),
  spanProcessors: [new SimpleSpanProcessor(exporter)],
});
const span = provider.getTracer("amber/otel-integration").startSpan("process-order");
const provenance = Provenance.start();

setProvenanceAttributes(span, provenance);
span.end();

const finished = exporter.getFinishedSpans();
assert(finished.length === 1, `recorded ${finished.length} finished spans, want 1`);
assert(
  finished[0].attributes["amber.execution_id"] === provenance.execution_id,
  "execution ID was not recorded by the SDK",
);
assert(finished[0].attributes["amber.depth"] === 0, "depth was not recorded by the SDK");
assert(
  finished[0].attributes["amber.mode.kind"] === "normal",
  "mode was not recorded by the SDK",
);

await provider.shutdown();
console.log("TypeScript OpenTelemetry SDK integration test passed");
