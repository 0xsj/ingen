import {
  AlwaysOnSampler,
  BasicTracerProvider,
  InMemorySpanExporter,
  SimpleSpanProcessor,
} from "@opentelemetry/sdk-trace-base";
import { Provenance } from "./provenance.js";
import { setProvenanceAttributes } from "./otel.js";

const exporter = new InMemorySpanExporter();
const provider = new BasicTracerProvider({
  sampler: new AlwaysOnSampler(),
  spanProcessors: [new SimpleSpanProcessor(exporter)],
});

const span = provider.getTracer("amber/otel-example").startSpan("process-order");
const provenance = Provenance.start();
setProvenanceAttributes(span, provenance);
span.end();

const finished = exporter.getFinishedSpans();
if (finished.length !== 1) {
  throw new Error(`recorded ${finished.length} finished spans, want 1`);
}
if (finished[0].attributes["amber.execution_id"] !== provenance.execution_id) {
  throw new Error("Amber execution attribute was not recorded");
}

console.log("finished spans: 1");
console.log("Amber execution attribute present: true");
await provider.shutdown();
