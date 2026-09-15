import { AmberTransitionError, Provenance } from "./provenance.js";
import { TraceAttributes, toTraceAttributes } from "./tracing.js";

/**
 * The span surface Amber needs from an OpenTelemetry-compatible span.
 * Structural typing keeps @opentelemetry/api optional for library consumers.
 */
export type OpenTelemetrySpanLike = {
  setAttributes(attributes: TraceAttributes): unknown;
};

/** Add Amber's tracing projection to an existing OpenTelemetry-compatible span. */
export function setProvenanceAttributes(
  span: OpenTelemetrySpanLike,
  provenance: Provenance,
): void {
  if (span === null || span === undefined || typeof span.setAttributes !== "function") {
    throw new AmberTransitionError("OpenTelemetry span must provide setAttributes");
  }
  span.setAttributes(toTraceAttributes(provenance));
}
