import {
  AmberTransitionError,
  Provenance,
} from "./provenance.js";
import {
  IncomingContextResult,
  IncomingInspection,
  IncomingPolicy,
} from "./incoming.js";
import { ProvenanceContext } from "./context.js";
import {
  MAX_ENCODED_VALUE_BYTES,
  PROVENANCE_FIELD,
  decodeValue,
  encodeValue,
} from "./transport.js";

export const AMBER_PROVENANCE_HEADER = PROVENANCE_FIELD;
export const MAX_ENCODED_HEADER_BYTES = MAX_ENCODED_VALUE_BYTES;

/** Encode a provenance value as unpadded base64url of UTF-8 canonical JSON. */
export function encodeHeader(provenance: Provenance): string {
  return encodeValue(provenance);
}

/** Decode and inspect an Amber HTTP header without installing it. */
export function decodeHeader(
  value: string | null | undefined,
  policy: IncomingPolicy,
): IncomingInspection {
  validatePolicy(policy);
  if (value === null || value === undefined || value.length === 0) {
    return { present: false };
  }
  return decodeValue(value, policy);
}

/** Inspect and install the header value into a derived context. */
export function withIncomingHeaders(
  context: ProvenanceContext,
  headers: Headers,
  policy: IncomingPolicy,
): IncomingContextResult {
  if (!(context instanceof ProvenanceContext)) {
    throw new AmberTransitionError("context must be a ProvenanceContext instance");
  }
  if (!(headers instanceof Headers)) {
    throw new AmberTransitionError("headers must be a Headers instance");
  }
  const inspected = decodeHeader(headers.get(AMBER_PROVENANCE_HEADER), policy);
  if (!inspected.present || inspected.provenance === undefined) {
    return { context, present: false };
  }
  return {
    context: context.withProvenance(inspected.provenance),
    present: true,
  };
}

export type IncomingRequestResult = {
  readonly request: Request;
  readonly context: ProvenanceContext;
  readonly present: boolean;
};

/** Inspect a Fetch-compatible request without mutating the request object. */
export function withIncomingRequest(
  request: Request,
  context: ProvenanceContext,
  policy: IncomingPolicy,
): IncomingRequestResult {
  if (!(request instanceof Request)) {
    throw new AmberTransitionError("request must be a Request instance");
  }
  const result = withIncomingHeaders(context, request.headers, policy);
  return { request, context: result.context, present: result.present };
}

/** Set the outgoing header on a mutable Fetch-compatible header collection. */
export function setOutgoingHeader(headers: Headers, provenance: Provenance): void {
  if (!(headers instanceof Headers)) {
    throw new AmberTransitionError("headers must be a Headers instance");
  }
  headers.set(AMBER_PROVENANCE_HEADER, encodeHeader(provenance));
}

/** Clone a Fetch-compatible request and add provenance to the clone. */
export function withOutgoingRequest(request: Request, provenance: Provenance): Request {
  if (!(request instanceof Request)) {
    throw new AmberTransitionError("request must be a Request instance");
  }
  const headers = new Headers(request.headers);
  setOutgoingHeader(headers, provenance);
  return new Request(request, { headers });
}

function validatePolicy(policy: IncomingPolicy): void {
  if (policy !== "reject" && policy !== "ignore") {
    throw new AmberTransitionError(`unsupported incoming policy ${String(policy)}`);
  }
}
