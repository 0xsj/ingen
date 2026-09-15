import {
  AmberTransitionError,
  Provenance,
} from "./provenance.js";
import {
  IncomingContextResult,
  IncomingInspection,
  IncomingPolicy,
  IncomingValidator,
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
  return decodeHeaderWithValidator(value, policy);
}

/** Decode an HTTP header and apply an optional trust validator. */
export function decodeHeaderWithValidator(
  value: string | null | undefined,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): IncomingInspection {
  validatePolicy(policy);
  if (value === null || value === undefined || value.length === 0) {
    return { present: false };
  }
  const inspected = decodeValue(value, policy);
  if (!inspected.present || inspected.provenance === undefined || validator === undefined) {
    return inspected;
  }
  try {
    validator(inspected.provenance);
  } catch (error) {
    if (policy === "ignore") {
      return { present: false };
    }
    throw new AmberTransitionError(`incoming validation failed: ${String(error)}`);
  }
  return inspected;
}

/** Inspect and install the header value into a derived context. */
export function withIncomingHeaders(
  context: ProvenanceContext,
  headers: Headers,
  policy: IncomingPolicy,
): IncomingContextResult {
  return withIncomingHeadersWithValidator(context, headers, policy);
}

/** Install an HTTP header only after an optional trust validator accepts it. */
export function withIncomingHeadersWithValidator(
  context: ProvenanceContext,
  headers: Headers,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): IncomingContextResult {
  if (!(context instanceof ProvenanceContext)) {
    throw new AmberTransitionError("context must be a ProvenanceContext instance");
  }
  if (!(headers instanceof Headers)) {
    throw new AmberTransitionError("headers must be a Headers instance");
  }
  const inspected = decodeHeaderWithValidator(headers.get(AMBER_PROVENANCE_HEADER), policy, validator);
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
  return withIncomingRequestWithValidator(request, context, policy);
}

/** Derive a request context after an optional trust validator accepts its header. */
export function withIncomingRequestWithValidator(
  request: Request,
  context: ProvenanceContext,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): IncomingRequestResult {
  if (!(request instanceof Request)) {
    throw new AmberTransitionError("request must be a Request instance");
  }
  const result = withIncomingHeadersWithValidator(context, request.headers, policy, validator);
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

/** Clone a Fetch-compatible response and add provenance to its headers. */
export function withOutgoingResponse(response: Response, provenance: Provenance): Response {
  if (!(response instanceof Response)) {
    throw new AmberTransitionError("response must be a Response instance");
  }
  const headers = new Headers(response.headers);
  setOutgoingHeader(headers, provenance);
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}

export type HTTPHandler = (
  request: Request,
  context: ProvenanceContext,
) => Response | Promise<Response>;

export type IncomingErrorHandler = (
  error: unknown,
  request: Request,
) => Response | Promise<Response>;

/** Return a generic HTTP 400 response for rejected incoming provenance. */
export function defaultIncomingErrorResponse(): Response {
  return new Response("invalid Amber provenance\n", {
    status: 400,
    headers: { "content-type": "text/plain; charset=UTF-8" },
  });
}

/**
 * Build a Fetch-compatible handler that owns the Amber HTTP boundary. The
 * handler receives a derived context and the response carries that same
 * provenance when one is present, unless the handler explicitly sets a
 * response provenance header for a child value.
 */
export function httpMiddleware(
  next: HTTPHandler,
  policy: IncomingPolicy,
  onError: IncomingErrorHandler = () => defaultIncomingErrorResponse(),
): HTTPHandler {
  return httpMiddlewareWithValidator(next, policy, undefined, onError);
}

/** Build HTTP middleware with a synchronous trust validator for incoming values. */
export function httpMiddlewareWithValidator(
  next: HTTPHandler,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
  onError: IncomingErrorHandler = () => defaultIncomingErrorResponse(),
): HTTPHandler {
  if (typeof next !== "function") {
    throw new AmberTransitionError("next handler must be a function");
  }

  return async (request, context) => {
    let incoming: IncomingRequestResult;
    try {
      incoming = withIncomingRequestWithValidator(request, context, policy, validator);
    } catch (error) {
      return onError(error, request);
    }

    const response = await next(incoming.request, incoming.context);
    if (
      incoming.context.provenance === undefined ||
      response.headers.has(AMBER_PROVENANCE_HEADER)
    ) {
      return response;
    }
    return withOutgoingResponse(response, incoming.context.provenance);
  };
}

function validatePolicy(policy: IncomingPolicy): void {
  if (policy !== "reject" && policy !== "ignore") {
    throw new AmberTransitionError(`unsupported incoming policy ${String(policy)}`);
  }
}
