import {
  AmberTransitionError,
  AmberValidationError,
  Provenance,
} from "./provenance.js";
import { ProvenanceContext } from "./context.js";

export const MAX_INCOMING_JSON_BYTES = 16 * 1024;

export type IncomingPolicy = "reject" | "ignore";

export type IncomingInspection = {
  readonly provenance?: Provenance;
  readonly present: boolean;
};

export type IncomingContextResult = {
  readonly context: ProvenanceContext;
  readonly present: boolean;
};

function validatePolicy(policy: IncomingPolicy): void {
  if (policy !== "reject" && policy !== "ignore") {
    throw new AmberTransitionError(`unsupported incoming policy ${String(policy)}`);
  }
}

function absent(): IncomingInspection {
  return { present: false };
}

function invalid(policy: IncomingPolicy, error: unknown): IncomingInspection {
  if (policy === "ignore") {
    return absent();
  }
  if (error instanceof AmberValidationError) {
    throw error;
  }
  throw new AmberValidationError(String(error));
}

/** Inspect incoming JSON without installing it into a context. */
export function inspectIncomingJSON(
  input: string | null | undefined,
  policy: IncomingPolicy,
): IncomingInspection {
  validatePolicy(policy);
  if (input === undefined || input === null || input.length === 0) {
    return absent();
  }
  if (typeof input !== "string") {
    return invalid(policy, new Error("incoming JSON must be a string"));
  }
  if (new TextEncoder().encode(input).byteLength > MAX_INCOMING_JSON_BYTES) {
    return invalid(
      policy,
      new Error(`incoming JSON exceeds ${MAX_INCOMING_JSON_BYTES} bytes`),
    );
  }
  try {
    return { provenance: Provenance.fromJSON(input), present: true };
  } catch (error) {
    return invalid(policy, error);
  }
}

/**
 * Inspect and, when accepted, install incoming JSON in a derived context.
 * Absent or ignored input returns the original context unchanged.
 */
export function withIncomingJSON(
  context: ProvenanceContext,
  input: string | null | undefined,
  policy: IncomingPolicy,
): IncomingContextResult {
  if (!(context instanceof ProvenanceContext)) {
    throw new AmberTransitionError("context must be a ProvenanceContext instance");
  }
  const inspected = inspectIncomingJSON(input, policy);
  if (!inspected.present || inspected.provenance === undefined) {
    return { context, present: false };
  }
  return {
    context: context.withProvenance(inspected.provenance),
    present: true,
  };
}
