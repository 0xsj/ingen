import { AmberTransitionError, Provenance } from "./provenance.js";

export type TraceAttributes = Readonly<Record<string, string | number>>;

/** Return the default tracing attribute projection for provenance. */
export function toTraceAttributes(provenance: Provenance): TraceAttributes {
  if (!(provenance instanceof Provenance)) {
    throw new AmberTransitionError("trace value must be a Provenance instance");
  }
  provenance.validate();
  const attributes: Record<string, string | number> = {
    "amber.version": provenance.version,
    "amber.work_id": provenance.work_id,
    "amber.execution_id": provenance.execution_id,
    "amber.correlation_id": provenance.correlation_id,
    "amber.origin": provenance.origin,
    "amber.depth": provenance.depth,
    "amber.attempt": provenance.attempt,
    "amber.mode.kind": provenance.mode.kind,
  };
  const mode = provenance.mode;
  if (mode.kind === "retry" || mode.kind === "replay") {
    attributes["amber.mode.of_execution_id"] = mode.of_execution_id;
  }
  if (mode.kind === "replay") {
    attributes["amber.mode.replay_id"] = mode.replay_id;
  }
  const causation = provenance.causation;
  if (causation !== undefined) {
    attributes["amber.causation.kind"] = causation.kind;
    attributes["amber.causation.id"] = causation.id;
    if (causation.type !== undefined) {
      attributes["amber.causation.type"] = causation.type;
    }
  }
  return Object.freeze(attributes);
}

/** Return cloned trace attributes with Amber attributes applied. */
export function mergeTraceAttributes(
  existing: Readonly<Record<string, unknown>>,
  provenance: Provenance,
): Record<string, unknown> {
  return { ...existing, ...toTraceAttributes(provenance) };
}
