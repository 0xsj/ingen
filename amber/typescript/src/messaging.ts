import { AmberTransitionError, Provenance } from "./provenance.js";
import {
  IncomingContextResult,
  IncomingPolicy,
} from "./incoming.js";
import { ProvenanceContext } from "./context.js";
import {
  PROVENANCE_FIELD,
  decodeValue,
  encodeValue,
} from "./transport.js";

export const PROVENANCE_METADATA_KEY = PROVENANCE_FIELD;

export type MessageMetadata = Readonly<Record<string, string>>;

/** Inspect message metadata without installing it into a context. */
export function decodeMetadata(
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
) {
  return decodeValue(metadata?.[PROVENANCE_METADATA_KEY], policy);
}

/** Inspect and install message metadata into a derived context. */
export function withIncomingMetadata(
  context: ProvenanceContext,
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
): IncomingContextResult {
  if (!(context instanceof ProvenanceContext)) {
    throw new AmberTransitionError("context must be a ProvenanceContext instance");
  }
  const inspected = decodeMetadata(metadata, policy);
  if (!inspected.present || inspected.provenance === undefined) {
    return { context, present: false };
  }
  return {
    context: context.withProvenance(inspected.provenance),
    present: true,
  };
}

/** Return cloned message metadata containing provenance; never mutate input. */
export function withOutgoingMetadata(
  metadata: MessageMetadata | null | undefined,
  provenance: Provenance,
): Record<string, string> {
  return {
    ...(metadata ?? {}),
    [PROVENANCE_METADATA_KEY]: encodeValue(provenance),
  };
}
