import { AmberTransitionError, Provenance } from "./provenance.js";

export type LogFields = Readonly<Record<string, string | number>>;

/** Return the default structured-log projection for provenance. */
export function toLogFields(provenance: Provenance): LogFields {
  if (!(provenance instanceof Provenance)) {
    throw new AmberTransitionError("log value must be a Provenance instance");
  }
  provenance.validate();
  const fields: Record<string, string | number> = {
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
    fields["amber.mode.of_execution_id"] = mode.of_execution_id;
  }
  if (mode.kind === "replay") {
    fields["amber.mode.replay_id"] = mode.replay_id;
  }
  const causation = provenance.causation;
  if (causation !== undefined) {
    fields["amber.causation.kind"] = causation.kind;
    fields["amber.causation.id"] = causation.id;
    if (causation.type !== undefined) {
      fields["amber.causation.type"] = causation.type;
    }
  }
  return Object.freeze(fields);
}

/** Return cloned log fields with Amber fields applied; never mutate input. */
export function mergeLogFields(
  existing: Readonly<Record<string, unknown>>,
  provenance: Provenance,
): Record<string, unknown> {
  return { ...existing, ...toLogFields(provenance) };
}
