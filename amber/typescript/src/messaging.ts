import { AmberTransitionError, Provenance } from "./provenance.js";
import {
  IncomingContextResult,
  IncomingPolicy,
  IncomingValidator,
} from "./incoming.js";
import { ProvenanceContext } from "./context.js";
import {
  PROVENANCE_FIELD,
  decodeValue,
  encodeValue,
} from "./transport.js";

export const PROVENANCE_METADATA_KEY = PROVENANCE_FIELD;

export type MessageMetadata = Readonly<Record<string, string>>;

/** A broker-neutral message whose body is opaque to Amber. */
export type Message<T = unknown> = {
  readonly body: T;
  readonly metadata?: MessageMetadata | null;
};

/** Process a message with its scoped provenance context. */
export type MessageHandler<T = unknown> = (
  message: Message<T>,
  context: ProvenanceContext,
) => Message<T> | Promise<Message<T>>;

/** Inspect message metadata without installing it into a context. */
export function decodeMetadata(
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
) {
  return decodeMetadataWithValidator(metadata, policy);
}

/** Decode message metadata and apply an optional trust validator. */
export function decodeMetadataWithValidator(
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
) {
  const inspected = decodeValue(metadata?.[PROVENANCE_METADATA_KEY], policy);
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

/** Inspect and install message metadata into a derived context. */
export function withIncomingMetadata(
  context: ProvenanceContext,
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
): IncomingContextResult {
  return withIncomingMetadataWithValidator(context, metadata, policy);
}

/** Install message metadata only after an optional trust validator accepts it. */
export function withIncomingMetadataWithValidator(
  context: ProvenanceContext,
  metadata: MessageMetadata | null | undefined,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): IncomingContextResult {
  if (!(context instanceof ProvenanceContext)) {
    throw new AmberTransitionError("context must be a ProvenanceContext instance");
  }
  const inspected = decodeMetadataWithValidator(metadata, policy, validator);
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

export type IncomingMessageResult<T> = {
  readonly message: Message<T>;
  readonly context: ProvenanceContext;
  readonly present: boolean;
};

/** Install message provenance and clone metadata before handing it to a consumer. */
export function withIncomingMessage<T>(
  message: Message<T>,
  context: ProvenanceContext,
  policy: IncomingPolicy,
): IncomingMessageResult<T> {
  return withIncomingMessageWithValidator(message, context, policy);
}

/** Install message provenance after an optional trust validator accepts it. */
export function withIncomingMessageWithValidator<T>(
  message: Message<T>,
  context: ProvenanceContext,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): IncomingMessageResult<T> {
  validateMessage(message);
  const result = withIncomingMetadataWithValidator(context, message.metadata, policy, validator);
  return {
    message: {
      ...message,
      metadata:
        message.metadata === null || message.metadata === undefined
          ? message.metadata
          : { ...message.metadata },
    },
    context: result.context,
    present: result.present,
  };
}

/** Return a message with cloned metadata containing provenance. */
export function withOutgoingMessage<T>(
  message: Message<T>,
  provenance: Provenance,
): Message<T> {
  validateMessage(message);
  return {
    ...message,
    metadata: withOutgoingMetadata(message.metadata, provenance),
  };
}

/**
 * Build a broker-neutral message handler with Amber context installation and
 * outgoing metadata propagation. Rejected input throws before next runs.
 */
export function messageMiddleware<T>(
  next: MessageHandler<T>,
  policy: IncomingPolicy,
): MessageHandler<T> {
  return messageMiddlewareWithValidator(next, policy);
}

/** Build message middleware with a synchronous trust validator. */
export function messageMiddlewareWithValidator<T>(
  next: MessageHandler<T>,
  policy: IncomingPolicy,
  validator?: IncomingValidator,
): MessageHandler<T> {
  if (typeof next !== "function") {
    throw new AmberTransitionError("next handler must be a function");
  }

  return async (message, context) => {
    const incoming = withIncomingMessageWithValidator(message, context, policy, validator);
    const outgoing = await next(incoming.message, incoming.context);
    if (incoming.context.provenance === undefined) {
      return outgoing;
    }
    return withOutgoingMessage(outgoing, incoming.context.provenance);
  };
}

function validateMessage<T>(message: Message<T>): void {
  if (message === null || typeof message !== "object") {
    throw new AmberTransitionError("message must be an object");
  }
}
